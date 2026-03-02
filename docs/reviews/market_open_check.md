# 코드 리뷰: 장운영일 체크 + Order Sync 스케줄러 최적화

> 작성일: 2026-03-02
> 대상 파일: `backend/internal/kis/client.go`, `backend/internal/agent/market.go`, `backend/internal/agent/history.go`, `backend/internal/api/handlers.go`, `backend/internal/api/router.go`

---

## 개요 (Overview)

이번 변경은 두 가지 문제를 해결합니다.

1. **불필요한 KIS API 호출 제거:** `StartOrderSyncScheduler`가 주말/공휴일에도 3분마다 KIS API를 호출하고 있었습니다.
2. **공휴일 무시 문제 해결:** 2026-03-02 (삼일절 대체공휴일)처럼 평일이지만 장이 쉬는 날에도 에이전트가 계속 작동하는 버그.

**해결 방향:** KIS `CTCA0903R` API로 영업일 여부를 조회하고, 결과를 하루 1회 캐시. 스케줄러는 매 틱마다 `IsMarketOpen()` 체크 후 장 마감이면 실행을 건너뜁니다.

---

## Go 백엔드 해설

### 1. `kis/client.go` — `GetMarketHolidayInfo()` 메서드

```go
type HolidayInfo struct {
    BassDate string `json:"bass_dt"` // YYYYMMDD
    IsBizDay string `json:"bzdy_yn"` // Y=영업일, N=휴장
}
```

**구조체 태그 (`json:"..."`):** Go의 JSON 직렬화/역직렬화 지시어입니다. KIS API 응답 필드명(`bass_dt`, `bzdy_yn`)을 Go 구조체 필드(`BassDate`, `IsBizDay`)에 매핑합니다. Go에서는 외부 API 필드명이 snake_case여도 구조체는 PascalCase로 짓는 관습이 있어 태그로 연결합니다.

```go
func (c *Client) GetMarketHolidayInfo(ctx context.Context, date string) (*HolidayInfo, error) {
    // ...
    var result struct {
        Output []HolidayInfo `json:"output"`
        RtCd   string        `json:"rt_cd"`
    }
```

**익명 구조체 (`var result struct {...}`):** 이 함수 안에서만 사용할 일회용 구조체입니다. 재사용할 필요가 없으면 타입을 별도로 선언하지 않고 이렇게 인라인으로 정의합니다. KIS API 응답의 최상위 봉투(envelope)를 파싱하는 데 자주 쓰이는 패턴입니다.

`output`이 배열인 이유: CTCA0903R은 기준일 이후의 영업일 목록을 반환합니다. 우리는 첫 번째 항목(`result.Output[0]`)만 사용합니다.

---

### 2. `agent/market.go` — `IsMarketOpen()` + 캐시

#### 패키지 레벨 변수 초기화

```go
var kstLocation = func() *time.Location {
    loc, err := time.LoadLocation("Asia/Seoul")
    if err != nil {
        loc = time.FixedZone("KST", 9*60*60)
    }
    return loc
}()
```

**즉시 실행 함수 (IIFE, Immediately Invoked Function Expression):** Go에서 패키지 레벨 변수를 복잡한 로직으로 초기화할 때 쓰는 패턴입니다. `var x = func() T { ... }()` — 함수를 정의하고 즉시 호출합니다. 서버 시작 시 딱 한 번만 실행됩니다.

`time.LoadLocation("Asia/Seoul")`은 시스템의 tzdata(타임존 데이터베이스)가 필요합니다. Alpine Linux 등 최소 이미지에는 tzdata가 없어 실패할 수 있어, 이 경우 `time.FixedZone("KST", 9*60*60)` 으로 UTC+9를 수동 설정합니다. `main.go`에 `import _ "time/tzdata"` 를 추가해 Go 바이너리 안에 tzdata를 내장하는 방식으로도 해결했습니다.

#### 뮤텍스를 사용한 캐시

```go
type marketDayCache struct {
    mu       sync.RWMutex
    date     string
    isBizDay bool
}

var pkgCache marketDayCache
```

**`sync.RWMutex` (읽기-쓰기 뮤텍스):** 여러 고루틴이 동시에 캐시를 읽을 수 있지만, 쓸 때는 단 하나의 고루틴만 허용합니다. 일반 `sync.Mutex`와 달리 읽기는 동시에 가능해 성능이 좋습니다.

```go
pkgCache.mu.RLock()   // 읽기 잠금 (다른 읽기와 공존 가능)
defer pkgCache.mu.RUnlock()
// ...
pkgCache.mu.Lock()    // 쓰기 잠금 (완전 독점)
pkgCache.date, pkgCache.isBizDay = today, isBiz
pkgCache.mu.Unlock()
```

**캐시 만료 전략:** `pkgCache.date`가 오늘 날짜(`20060102` 포맷)와 다르면 KIS API를 호출하고 결과를 저장합니다. 자정이 지나면 날짜가 달라지므로 다음 날 첫 틱에서 자동으로 갱신됩니다. 별도의 타이머나 TTL 없이 날짜 문자열 비교만으로 해결한 단순하면서도 효과적인 패턴입니다.

#### 시간 범위 체크

```go
openMinute := now.Hour()*60 + now.Minute()
if openMinute < 9*60 || openMinute >= 15*60+30 {
    return false, nil
}
```

시각을 "분 단위 정수"로 변환해 비교합니다. `9:00 = 540`, `15:30 = 930`. `now.Hour()*60 + now.Minute()` 가 540 미만이거나 930 이상이면 장 외 시간입니다. `>=` 를 쓰는 이유: 15시 30분 00초부터는 장이 닫히므로 **포함하지 않습니다**.

---

### 3. `agent/history.go` — 스케줄러 가드

```go
case <-ticker.C:
    open, err := IsMarketOpen(ctx, client)
    if err != nil {
        logger.Warn("market status check failed — skipping sync", ...)
        continue   // ← 이번 틱을 건너뛰고 다음 틱을 기다림
    }
    if !open {
        logger.Info("market closed — skipping order sync", nil)
        continue
    }
    // 기존 동기화 로직...
```

**`select` + `continue`:** Go의 `select` 문은 채널 연산을 기다립니다. `ticker.C`는 3분마다 신호를 보내는 채널입니다. `continue`는 `for` 루프의 다음 이터레이션으로 넘어가 다시 `select`에서 대기합니다. 즉, 현재 틱의 처리를 포기하고 다음 신호를 기다립니다.

**Fail-safe (안전 기본값):** `IsMarketOpen`이 에러를 반환하면 `continue`로 건너뜁니다. KIS API에 문제가 있을 때 동기화를 시도해 추가 에러를 쌓지 않는 보수적인 전략입니다.

---

### 4. `api/handlers.go` — `GetMarketStatus` 핸들러

```go
func (h *Handler) GetMarketStatus(c *gin.Context) {
    now := time.Now().In(agent.KSTLocation())
    checkedAt := now.Format(time.RFC3339)

    if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
        c.JSON(http.StatusOK, gin.H{"is_open": false, ..., "reason": "weekend"})
        return
    }
    // ...
    isOpen, err := agent.IsMarketOpen(c.Request.Context(), h.client)
```

**`if` 초기화 문 (`if wd := ...; condition`):** Go에서 `if` 조건 앞에 짧은 변수 선언을 할 수 있습니다. `wd`는 이 `if` 블록 스코프 안에서만 유효합니다. 변수 범위를 최소화하는 Go 관용구입니다.

핸들러가 주말/시간 체크를 직접 수행하는 이유: `IsMarketOpen()`은 캐시 결과(bool)만 반환해 "왜 닫혔는지" 이유를 알 수 없습니다. 핸들러에서 단계별로 체크하면 `reason` 필드에 구체적인 이유를 담을 수 있습니다.

---

## 핵심 요약 (Key Takeaways)

| 개념 | 설명 |
|------|------|
| **패키지 레벨 캐시** | `var pkgCache = ...` + `sync.RWMutex` 조합으로 고루틴 안전한 싱글톤 캐시 구현 |
| **IIFE 초기화** | `var x = func() T { ... }()` — 복잡한 초기화 로직을 패키지 레벨 변수에 적용 |
| **RWMutex vs Mutex** | 읽기 多·쓰기 少인 캐시에는 `RWMutex`가 성능상 유리 |
| **분 단위 시간 비교** | `hour*60 + minute` 변환으로 시간 범위 비교를 단순화 |
| **Fail-safe 원칙** | API 오류 시 `false` 반환 → 불확실한 상황에서 거래/호출을 막는 보수적 전략 |
| **`time/tzdata` 임베딩** | `import _ "time/tzdata"` 로 바이너리에 타임존 데이터 내장 → 최소 서버 이미지 호환 |
| **`continue` in goroutine** | 스케줄러 루프에서 조건 불충족 시 `continue`로 현재 틱을 건너뜀 |
