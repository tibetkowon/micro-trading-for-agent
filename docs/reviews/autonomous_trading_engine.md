# 자율 트레이딩 엔진 코드 리뷰

> 작성일: 2026-03-06
> 대상 파일: `backend/internal/trader/claude.go`, `backend/internal/trader/engine.go`, `backend/internal/monitor/monitor.go`, `backend/cmd/server/main.go`, `frontend/src/pages/Reports.jsx`, `frontend/src/pages/Settings.jsx`

---

## 개요

외부 AI 에이전트가 HTTP API를 폴링하며 매매를 지시하던 구조에서, **서버 자체가 Claude API를 호출하여 종목 선정부터 매수·모니터링·매도·리포트까지 완전 자율 수행**하는 구조로 전환한 작업입니다.

---

## Go 백엔드 해설

### 1. 상태 머신 (State Machine) — `engine.go`

```go
type EngineState string

const (
    StateIdle        EngineState = "IDLE"
    StateSelecting   EngineState = "SELECTING"
    StateOrdering    EngineState = "ORDERING"
    StateWaitingFill EngineState = "WAITING_FILL"
    StateMonitoring  EngineState = "MONITORING"
)
```

**상태 머신**이란 시스템이 특정 상태들 중 하나에만 있을 수 있고, 조건에 따라 다음 상태로 전이하는 패턴입니다.
Go에서는 `string` 기반 타입(`EngineState`)을 정의해 상태를 표현합니다. `const` 블록으로 허용된 값을 열거하여 오타를 방지합니다.

```go
func (e *Engine) setState(s EngineState) {
    e.mu.Lock()
    e.state = s
    e.mu.Unlock()
    logger.Info("engine: state changed", map[string]any{"state": string(s)})
}
```

`sync.RWMutex`를 사용하는 이유: 엔진 고루틴과 HTTP 핸들러가 동시에 `state`를 읽을 수 있어서 **경쟁 조건(race condition)**이 발생할 수 있습니다. `mu.Lock()`으로 쓰기를 직렬화하고, `mu.RLock()`으로 읽기는 동시에 허용합니다.

---

### 2. 고루틴과 채널 패턴 — `engine.go`

```go
type Engine struct {
    soldCh chan string    // 매도 완료 시 stock_code 수신
    stopCh chan struct{}
}
```

**채널(channel)**은 고루틴 간 통신 수단입니다.

- `soldCh chan string`: Monitor가 매도를 실행하면 종목 코드를 이 채널로 전송 → 엔진이 "다음 종목 선정 가능" 신호로 사용
- `stopCh chan struct{}`: `struct{}{}`는 데이터 없이 신호만 전달할 때 관용적으로 쓰는 패턴. `close(stopCh)`로 모든 대기 중인 `select` 블록에 동시에 신호를 줄 수 있습니다.

```go
func (e *Engine) Start(ctx context.Context) func() {
    e.stopCh = make(chan struct{})
    go e.runCycle(ctx)
    return func() { close(e.stopCh) }
}
```

`Start()`가 **stop 함수를 반환**하는 패턴: 호출자가 반환된 함수를 변수에 저장했다가 엔진을 멈출 때 호출합니다. `defer` 없이도 명시적으로 정리할 수 있어 스케줄러에서 유용합니다.

---

### 3. ExecCh drain-and-match — `engine.go`

```go
func (e *Engine) waitForFill(ctx context.Context, kisOrderID string, timeout time.Duration) (float64, int, error) {
    deadline := time.After(timeout)
    for {
        select {
        case exec := <-e.wsClient.ExecCh:
            if exec.OrderNo == kisOrderID && exec.CntgYN == "2" && exec.SellBuyDiv == "02" {
                // 체결 확인
            }
        case <-deadline:
            return 0, 0, fmt.Errorf("fill timeout")
        case <-ctx.Done():
            return 0, 0, ctx.Err()
        }
    }
}
```

`select` 문은 여러 채널 중 먼저 값이 오는 케이스를 실행합니다. `time.After()`는 지정 시간 후 값을 보내는 채널을 반환하므로 타임아웃 구현에 자주 씁니다.
`ctx.Done()`을 함께 체크하면 서버 종료 시 무한 대기를 방지합니다.

---

### 4. 콜백 주입으로 순환 임포트 방지 — `monitor.go`

```go
func (m *Monitor) StartIndicatorChecker(
    ctx context.Context,
    intervalMin int,
    conditions []string,
    rsiThreshold float64,
    macdBearish bool,
    getInfoFn func(ctx context.Context, code string) (*IndicatorSnapshot, error),
)
```

`monitor` 패키지에서 `agent.GetStockInfo()`를 직접 호출하면 `monitor → agent → monitor` 형태의 **순환 임포트(circular import)**가 발생합니다. Go는 이를 컴파일 오류로 처리합니다.
해결책: 함수 타입(`func(ctx, code) (*IndicatorSnapshot, error)`)을 파라미터로 받아, `main.go`에서 실제 함수를 주입합니다. 패키지 경계를 넘지 않아 순환이 끊깁니다.

---

### 5. SoldCh 신호 패턴 — `monitor.go`

```go
type MonitoredEntry struct {
    // ...
    SoldCh chan<- string  // 단방향 송신 채널
}

// executeSell 완료 후
if pos.SoldCh != nil {
    select {
    case pos.SoldCh <- stockCode:
    default:  // 채널이 가득 차도 블로킹하지 않음
    }
}
```

`chan<- string`은 **송신 전용 채널**입니다. Monitor는 엔진의 `soldCh`에 쓰기만 할 수 있고 읽을 수 없습니다. 방향을 명시하면 코드 의도가 명확해지고 실수를 방지합니다.
`select { case ...: default: }` 패턴은 채널이 수신자 없이 가득 찬 경우 블로킹 없이 넘어가는 **non-blocking send**입니다.

---

### 6. Claude API 호출 — `claude.go`

```go
msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
    Model:     anthropic.Model(c.model),
    MaxTokens: 256,
    Messages: []anthropic.MessageParam{
        anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
    },
})
raw := msg.Content[0].AsText().Text
```

`anthropic-sdk-go`의 응답은 `Content` 슬라이스로 옵니다. `.AsText().Text`로 텍스트 블록을 추출합니다. 응답이 JSON이어야 하는 경우 markdown 코드 펜스(`\`\`\``)가 포함될 수 있어 전처리가 필요합니다.

```go
if strings.HasPrefix(raw, "```") {
    lines := strings.Split(raw, "\n")
    raw = strings.Join(lines[1:len(lines)-1], "\n")
}
```

---

### 7. 리포트 역할 분리 — `engine.go`

```go
// 서버가 직접 계산
for _, t := range trades {
    pnl := (o.FilledPrice - buy.FilledPrice) * float64(o.Qty)
    // ...
    sb.WriteString(fmt.Sprintf("| %s | %.0f | %.0f | ...\n", ...))
}

// Claude는 분석 텍스트만
analysis, _ := e.claude.GenerateReport(ctx, ReportSummary{
    TotalPnL: totalPnL, WinCount: winCount, ...
})
```

**역할 분리 원칙**: 계산 가능한 것(표, 손익)은 서버에서, 판단이 필요한 것(분석, 조언)만 Claude에게 맡깁니다. Claude 호출 비용(토큰, 레이턴시)을 최소화하고 표 계산 오류 가능성을 없앱니다.

---

## React 프론트엔드 해설

### Reports.jsx

```jsx
const [dates, setDates] = useState([])
const [selected, setSelected] = useState(null)
const [content, setContent] = useState('')

useEffect(() => {
    fetch('/api/reports').then(r => r.json()).then(data => setDates(data || []))
}, [])

useEffect(() => {
    if (!selected) return
    fetch(`/api/reports/${selected}`).then(r => r.json()).then(data => setContent(data.content || ''))
}, [selected])
```

`useEffect`를 두 개 분리한 이유: 의존성 배열(`[]` vs `[selected]`)로 실행 시점을 다르게 제어합니다.
- 첫 번째: 컴포넌트 마운트 시 한 번만 날짜 목록 로드
- 두 번째: `selected`가 바뀔 때마다 해당 날짜 리포트 내용 로드

```jsx
<pre className="whitespace-pre-wrap text-sm text-gray-300">
    {content}
</pre>
```

`whitespace-pre-wrap`: 줄바꿈과 공백을 유지하면서 창 너비에 맞게 줄바꿈합니다. 마크다운 텍스트를 별도 파서 없이 표시할 때 사용합니다.

### Settings.jsx — 체크박스 우선순위 배열 관리

```jsx
const toggleCondition = (val) => {
    setSellConditions(prev =>
        prev.includes(val) ? prev.filter(v => v !== val) : [...prev, val]
    )
}
```

`prev.includes(val)`: 이미 있으면 제거(`filter`), 없으면 추가(`[...prev, val]`). 스프레드 연산자(`...`)로 기존 배열을 복사해 불변성을 유지합니다.

---

## 핵심 요약

| 개념 | 적용 위치 | 핵심 |
|------|-----------|------|
| 상태 머신 | `engine.go` | `string` 타입 + `const` 열거로 안전한 상태 전이 |
| `sync.RWMutex` | `engine.go` | 고루틴 간 공유 상태 보호. 읽기 다수/쓰기 단일 패턴 |
| non-blocking send | `monitor.go` | `select { case: default: }` 로 채널 블로킹 방지 |
| 단방향 채널 `chan<-` | `monitor.go` | 의도 명시 + 오용 방지 |
| 콜백 주입 | `monitor.go` | 순환 임포트 방지의 Go 관용 패턴 |
| stop 함수 반환 | `engine.go` | `Start() func()` — 정리 책임을 호출자에게 위임 |
| 역할 분리 | `engine.go` | 계산은 서버, 판단은 Claude — 비용·신뢰성 최적화 |
