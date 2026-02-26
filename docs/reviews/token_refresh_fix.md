# 코드 리뷰: KIS 토큰 자동 갱신 버그 수정 (EGW00123)

> 작성일: 2026-02-26
> 대상 파일: `backend/internal/kis/token.go`, `backend/internal/kis/client.go`

---

## 개요

KIS API를 호출할 때 `{"rt_cd":"1","msg_cd":"EGW00123","msg1":"기간이 만료된 token 입니다."}` 오류가 지속되던 문제를 수정했습니다.
토큰은 24시간마다 만료되고, 시스템은 20시간마다 자동으로 갱신하도록 설계되어 있었지만 세 가지 버그로 인해 실제로는 동작하지 않았습니다.

---

## Go 백엔드 해설

### 버그 1: `time.NewTicker` vs `time.NewTimer` — 타이머 시작 시점 문제

**수정 전 코드 (`token.go`)**
```go
func (tm *TokenManager) StartAutoRefresh(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(tokenRefreshInterval) // 항상 지금부터 20h
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                tm.IssueToken(ctx)
            // ...
            }
        }
    }()
}
```

`time.NewTicker(d)`는 **지금 이 순간**부터 `d` 간격으로 채널에 신호를 보냅니다. 서버를 재시작할 때마다 20시간 타이머가 처음부터 다시 시작되는 것이 문제였습니다.

**수정 후 코드**
```go
// 1단계: DB에서 마지막 토큰의 발급 시각을 읽어 남은 시간 계산
firstDelay := tokenRefreshInterval
if tok, err := tm.GetCurrentToken(ctx); err == nil {
    elapsed := time.Since(tok.IssuedAt)
    if remaining := tokenRefreshInterval - elapsed; remaining > 0 {
        firstDelay = remaining
    } else {
        firstDelay = 0 // 이미 갱신 기한이 지남 → 즉시 재발급
    }
}

// 2단계: time.NewTimer로 1회성 첫 발동 처리
timer := time.NewTimer(firstDelay)
select {
case <-timer.C:
    tm.IssueToken(ctx)
case <-tm.stopCh:
    timer.Stop()
    return
}

// 3단계: 이후부터는 20h 고정 간격 ticker
ticker := time.NewTicker(tokenRefreshInterval)
```

**Go 개념 포인트:**
- `time.Since(t)` — 인자로 받은 시각 `t` 부터 현재까지 경과한 시간을 `time.Duration` 으로 반환
- `time.NewTimer(d)` — `d` 후 **딱 한 번** 채널에 신호를 보내는 1회성 타이머
- `time.NewTicker(d)` — `d` 간격으로 **반복적으로** 채널에 신호를 보내는 반복 타이머
- 두 가지를 조합해 "첫 번째는 남은 시간 후, 이후는 20시간 간격" 패턴을 구현

---

### 버그 2: `GetCurrentToken` vs `EnsureToken` — 만료 검사 누락

**수정 전 코드 (`client.go`)**
```go
func (c *Client) get(ctx context.Context, ...) ([]byte, error) {
    tok, err := c.tokenManager.GetCurrentToken(ctx) // 만료 여부 미확인
    // ...
}
```

`GetCurrentToken()`은 단순히 DB에서 가장 최근 토큰을 꺼내 반환합니다. 토큰이 이미 만료되어 있어도 그대로 반환하기 때문에 만료된 토큰이 API 헤더에 실려 나갔습니다.

**수정 후 코드**
```go
tok, err := c.tokenManager.EnsureToken(ctx) // 만료 임박 시 자동 재발급
```

`EnsureToken()`의 로직:
```go
func (tm *TokenManager) EnsureToken(ctx context.Context) (*models.Token, error) {
    tok, err := tm.GetCurrentToken(ctx)
    if err == nil && time.Until(tok.ExpiresAt) > time.Hour {
        return tok, nil // 1시간 이상 남아 있으면 재사용
    }
    return tm.IssueToken(ctx) // 1시간 미만이면 즉시 재발급
}
```

**Go 개념 포인트:**
- `time.Until(t)` — 현재 시각부터 미래 시각 `t`까지 남은 시간을 반환. `t`가 이미 지났으면 음수를 반환
- `time.Until(tok.ExpiresAt) > time.Hour` — "만료까지 1시간 이상 남았는가?" 를 한 줄로 표현
- 이 패턴은 **사전 예방(proactive)** 접근으로, 만료 직전에 미리 토큰을 갱신해 중단 없이 API를 호출할 수 있게 해줌

---

### 버그 3: HTTP 200인데 실패인 KIS 응답 패턴

**배경 지식:**

일반적인 REST API는 인증 실패 시 HTTP 401을 반환합니다. 하지만 KIS API는 특이하게도 **HTTP 200으로 응답하면서 본문 안에 에러 정보**를 담아 보냅니다.

```json
HTTP 200 OK
{
  "rt_cd": "1",       ← "0"이면 성공, "1"이면 실패
  "msg_cd": "EGW00123",
  "msg1": "기간이 만료된 token 입니다."
}
```

기존 코드는 HTTP 상태 코드만 확인했기 때문에 이 에러를 감지하지 못했습니다.

**수정 후 코드**
```go
// HTTP 200 확인 후 추가로 rt_cd 파싱
var envelope struct {
    RtCd    string `json:"rt_cd"`
    MsgCode string `json:"msg_cd"`
    Msg     string `json:"msg1"`
}
if err := json.Unmarshal(raw, &envelope); err == nil && envelope.RtCd == "1" {
    c.logAPIError(endpoint, envelope.MsgCode, string(raw))
    if envelope.MsgCode == "EGW00123" {
        // 즉시 새 토큰 발급 (다음 호출에서 사용)
        c.tokenManager.IssueToken(ctx)
    }
    return nil, fmt.Errorf("KIS error [%s]: %s", envelope.MsgCode, envelope.Msg)
}
```

**Go 개념 포인트:**
- **익명 구조체(anonymous struct)**: `var envelope struct { ... }` 처럼 타입 이름 없이 바로 선언. 한 곳에서만 사용할 JSON 파싱 대상을 위해 별도 타입을 만들 필요가 없을 때 유용
- `json.Unmarshal(raw, &envelope)` — JSON 바이트 슬라이스를 구조체로 변환. 성공하면 `nil`, 실패하면 에러 반환
- `&&` 단축 평가: `json.Unmarshal`이 성공(`err == nil`)한 경우에만 `envelope.RtCd == "1"` 을 검사

---

## 전체 토큰 갱신 흐름 (수정 후)

```
서버 시작
  ├─ EnsureToken() 호출 → 유효 토큰이 있으면 재사용, 없으면 발급
  └─ StartAutoRefresh() 실행
       ├─ DB에서 last_issued_at 읽기
       ├─ firstDelay = 20h - 경과시간
       ├─ time.NewTimer(firstDelay) → 정확한 시각에 첫 갱신
       └─ 이후 time.NewTicker(20h) → 반복 갱신

API 호출 (get / placeOrder)
  └─ EnsureToken() 호출
       ├─ 잔여 > 1h → 기존 토큰 재사용
       └─ 잔여 ≤ 1h → IssueToken() 새 토큰 발급

KIS 응답 수신 (get)
  ├─ HTTP != 200 → 에러
  ├─ rt_cd == "1" && msg_cd == "EGW00123"
  │    └─ IssueToken() 즉시 발급 (안전망)
  └─ 정상 데이터 반환
```

---

## 핵심 요약

| 패턴 | 설명 | 사용 위치 |
|------|------|-----------|
| `time.NewTimer` + `time.NewTicker` 조합 | 첫 발동은 특정 delay 후, 이후는 고정 간격 반복 | `StartAutoRefresh` |
| `time.Until(t)` | 미래 시각까지 남은 시간 계산 | `EnsureToken` |
| 익명 구조체로 JSON 파싱 | 일회성 파싱에 별도 타입 불필요 | `get()` 응답 파싱 |
| 사전 예방 + 사후 복구 이중 안전망 | 정기 갱신(proactive) + 에러 감지 즉시 갱신(reactive) | token.go + client.go |

> **기억할 점**: KIS API처럼 "HTTP 200인데 에러"인 패턴은 금융 API에서 종종 등장합니다. 항상 HTTP 상태 코드와 **응답 본문의 비즈니스 결과 코드**를 둘 다 확인해야 합니다.
