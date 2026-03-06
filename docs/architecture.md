# Project Architecture

> Last updated: 2026-03-06 (rev 7 — 자율 트레이딩 엔진 도입, Claude API 통합)

## Directory Tree

```
micro-trading-for-agent/
├── .github/
│   └── workflows/
│       └── ci.yml              # CI: Go build/test/fmt + React lint/build; CD: linux/amd64 크로스 컴파일 → SCP → rsync → systemctl restart
├── .claude/
│   └── skills/                 # AI 에이전트 행동 지침 파일 (.md)
├── backend/                    # Go backend root
│   ├── cmd/
│   │   └── server/
│   │       └── main.go         # 진입점; ClaudeClient·Engine 초기화, WebSocket·Monitor, 장운영 스케줄러, 서버 시작/종료
│   ├── internal/
│   │   ├── config/
│   │   │   └── config.go       # .env 로드 (godotenv); KIS·MQTT·Anthropic·서버 설정
│   │   ├── database/
│   │   │   └── db.go           # SQLite 초기화 + 스키마 마이그레이션; GetTradingSettings(), SaveReport()
│   │   ├── models/
│   │   │   └── models.go       # DB 테이블 1:1 Go 구조체 (Order, Report, MonitoredPosition 등)
│   │   ├── logger/
│   │   │   └── logger.go       # 구조화 JSON 로깅; KISError()는 필수 필드(error_code, timestamp, raw_response) 강제
│   │   ├── kis/
│   │   │   ├── client.go       # KIS REST API 클라이언트; 랭킹/주문/잔고/가격 조회
│   │   │   ├── websocket.go    # KIS WebSocket 클라이언트 (gorilla/websocket, AES-256-CBC); PriceCh/ExecCh
│   │   │   ├── chart.go        # KIS 차트 API: GetMinuteChart(1분봉), GetDailyChart(일봉)
│   │   │   ├── token.go        # OAuth 토큰 발급 + 20시간 자동 갱신 + 자격증명 지문 체크
│   │   │   └── ratelimiter.go  # TPS 리미터 (15 req/s, golang.org/x/time/rate)
│   │   ├── mqtt/
│   │   │   └── publisher.go    # Paho MQTT 퍼블리셔; PublishAlert() (목표가/손절가/청산 이벤트)
│   │   ├── monitor/
│   │   │   └── monitor.go      # 포지션 모니터; HandlePrice() 가격 체크 + 자동 매도, StartIndicatorChecker() RSI/MACD 주기 평가, LiquidateAll(), SoldCh 엔진 신호
│   │   ├── agent/
│   │   │   ├── market.go       # IsMarketOpen(): KST 평일·장 시간·KIS 영업일 3중 체크 + 당일 캐시
│   │   │   ├── stock_info.go   # GetStockInfo: 현재가 + MA5/MA20 + RSI14 + MACD(12,26,9)
│   │   │   ├── chart.go        # GetChart: OHLCV 캔들 (1m/5m/1h), 페이지네이션 + 집계
│   │   │   ├── balance.go      # GetAccountBalance: 잔고 조회 + DB 스냅샷 저장
│   │   │   ├── order.go        # PlaceOrder/CancelOrder/CheckOrderFeasibility
│   │   │   ├── ranking.go      # 거래량/체결강도/대량체결건수/이격도 순위 조회 래퍼
│   │   │   └── history.go      # KIS 체결 내역 동기화; StartOrderSyncScheduler (5분 ticker, 장중에만)
│   │   ├── trader/
│   │   │   ├── claude.go       # ClaudeClient: SelectStock() (JSON 파싱), GenerateReport() (한국어 마크다운)
│   │   │   └── engine.go       # Engine 상태 머신; 자율 종목 선정·주문·체결 대기·모니터 등록 사이클
│   │   └── api/
│   │       ├── handlers.go     # HTTP 핸들러: 잔고/종목/차트/주문/모니터/서버상태/순위/설정/리포트/로그/디버그
│   │       └── router.go       # gin.Engine 설정; 라우트 등록; SPA 폴백
│   ├── data/                   # SQLite .db 파일 (git-ignored)
│   └── go.mod                  # Go 모듈 정의
├── frontend/                   # React frontend root
│   ├── src/
│   │   ├── main.jsx            # React 진입점; BrowserRouter 설정
│   │   ├── App.jsx             # 루트 컴포넌트; 네비게이션 + 라우트 정의
│   │   ├── index.css           # Tailwind 베이스 스타일
│   │   ├── hooks/
│   │   │   └── useApi.js       # 범용 fetch 훅 (loading/error/data/refetch)
│   │   ├── components/
│   │   │   ├── Card.jsx        # 재사용 통계 카드
│   │   │   └── StatusBadge.jsx # 주문 상태 배지 (색상 코딩)
│   │   └── pages/
│   │       ├── Dashboard.jsx   # 잔고 카드 + 보유 종목 + 트레이더 상태
│   │       ├── Monitor.jsx     # 모니터링 포지션 목록
│   │       ├── Orders.jsx      # 주문 내역 테이블 (오늘 기준)
│   │       ├── KISLogs.jsx     # KIS API 에러 로그 뷰어
│   │       ├── Settings.jsx    # 거래 파라미터·순위·매도조건·지표·Claude 설정
│   │       ├── Reports.jsx     # 일일 리포트 목록 + 내용 표시
│   │       └── Debug.jsx       # WebSocket·Monitor·가격주입 테스트 도구
│   ├── index.html              # Vite HTML 템플릿
│   ├── vite.config.js          # Vite 설정; /api 프록시 → :8080
│   ├── tailwind.config.js      # Tailwind content 경로
│   ├── postcss.config.js       # PostCSS (Tailwind + autoprefixer)
│   └── package.json            # npm 의존성
├── docs/
│   ├── architecture.md         # 이 파일
│   ├── db_schema.md            # SQLite 테이블 스키마 문서
│   ├── changelog.md            # 변경 이력 (최신 항목이 맨 위)
│   ├── guides/
│   │   └── mqtt-setup.md       # MQTT 브로커 설치 및 사용자 알림 구독 가이드
│   ├── kis-api/                # KIS API 공식 명세서 (기본시세/순위분석/종목정보/주문계좌/인증/실시간)
│   ├── plans/                  # 기능 구현 계획 문서
│   └── reviews/                # 한국어 코드 리뷰 문서
├── SKILL.md                    # 에이전트 스킬 퀵 레퍼런스
├── .env.example                # 환경변수 템플릿 (시크릿 미포함)
├── .gitignore                  # .env, *.db, node_modules, 바이너리 제외
├── CLAUDE.md                   # AI 에이전트 프로젝트 지침
└── README.md                   # 프로젝트 개요
```

---

## Component Responsibilities

### `backend/internal/config`
- **Role:** 환경변수에서 모든 설정을 로드 (하드코딩 금지).
- **현재 관리 항목:** KIS 자격증명, Anthropic API 키, MQTT 브로커, HTS ID, 서버 포트, DB 경로

### `backend/internal/database`
- **Role:** SQLite 연결 초기화 및 서버 시작 시 스키마 마이그레이션 자동 실행.
- `GetTradingSettings(ctx)` — 자율 트레이딩 설정 12개를 일괄 조회하여 `TradingSettings` 구조체로 반환
- `SaveReport(ctx, date, content)` — 일일 리포트 upsert
- 신규 설정 키는 `INSERT OR IGNORE`로 기본값 자동 삽입 (서버 기동 시)

### `backend/internal/models`
- **Role:** DB 테이블과 1:1 대응하는 공유 데이터 구조체.
- `Order`, `MonitoredPosition`, `Balance`, `KISAPILog`, `Token`, `Report`, `Setting`

### `backend/internal/kis`
- **`client.go`** — REST API 요청/응답, 토큰 주입, Rate Limiting
- **`websocket.go`** — WebSocket 연결/재연결, `H0STCNT0`(체결가)/`H0STCNI0`(체결통보) 구독, AES-256-CBC 복호화
  - `PriceCh chan PriceEvent` — monitor.StartPriceConsumer가 소비
  - `ExecCh chan ExecEvent` — trader.Engine이 체결 확인에 사용 (단일 소비자)
- **`token.go`** — OAuth 토큰 발급·갱신·캐시, 자격증명 지문 체크

### `backend/internal/mqtt`
- **Role:** 사용자 알림 전용 MQTT 발행 레이어. 브로커 미연결 시 로그만 남기고 서버 기동 유지.
- `PublishAlert(event, stockCode, ...)` — TARGET_HIT / STOP_HIT / LIQUIDATION 알림

### `backend/internal/monitor`
- **Role:** 보유 포지션 실시간 모니터링.
- `Register(pos MonitoredEntry)` — 포지션 등록; `MonitoredEntry.SoldCh`에 엔진 채널 주입 가능
- `HandlePrice(stockCode, price, isTest)` — WebSocket 가격 이벤트 처리; 목표/손절 도달 시 `executeSell()` + MQTT + `SoldCh` 알림
- `StartIndicatorChecker(ctx, intervalMin, conditions, rsiThreshold, macdBearish, getInfoFn)` — RSI/MACD 주기 평가; `getInfoFn` 콜백으로 순환 임포트 방지
- `LiquidateAll(ctx)` — 15:15 전량 시장가 청산

### `backend/internal/agent`
- **Role:** KIS API 데이터와 DB를 연결하는 거래 액션 함수 모음.
- `IsMarketOpen()` — KST 평일·장 운영 시간(9:00~15:30)·KIS 영업일 여부 체크, 당일 캐시
- `GetStockInfo()` — 현재가 + MA5/MA20 + RSI14 + MACD(12,26,9) 계산
- `PlaceOrder()`, `CancelOrder()`, `CheckOrderFeasibility()` — 주문 실행 및 가능수량 조회
- `StartOrderSyncScheduler()` — 5분 간격 KIS 체결 내역 동기화

### `backend/internal/trader`
- **Role:** Claude API 기반 자율 트레이딩 엔진.
- **`claude.go`**
  - `ClaudeClient.SelectStock(ctx, rankings, availableCash, excludedCodes)` → `(stockCode, reason, error)`
  - `ClaudeClient.GenerateReport(ctx, date, trades, totalEval, withdrawable)` → `(markdown, error)`
- **`engine.go`**
  - `Engine.Start(ctx)` → 사이클 goroutine 시작, `stop func()` 반환
  - `Engine.GetState()` → 현재 상태 (`IDLE|SELECTING|ORDERING|WAITING_FILL|MONITORING`)
  - `Engine.GenerateDailyReport(ctx)` → 당일 AGENT 주문 로드 후 Claude 리포트 생성
  - `Engine.SoldCh()` → Monitor에 주입할 매도 완료 채널 반환

### `backend/internal/api`
- **Role:** HTTP 레이어. 입력 검증 → agent/db/engine 함수 호출 → JSON 응답.
- `Handler.SetEngine(e)` — main에서 engine 주입

---

## 장운영 스케줄러 (main.go `runMarketScheduler`)

```
08:50 (KST) → tokenManager.IssueToken()
            → kisClient.GetApprovalKey()
            → wsClient.StartWithReconnect()
            → wsClient.SubscribeExecNotice()

09:00 (KST) → db.GetSetting("trading_enabled") 확인
            → agent.IsMarketOpen() 확인
            → tradingReady = true (조건 충족 시)

09:15 (KST) → engine.Start(ctx)                  ← tradingReady == true 시에만
            → mon.StartIndicatorChecker(ctx, ...)

15:15 (KST) → engine stop()
            → mon.StartIndicatorChecker cancel()
            → mon.LiquidateAll(ctx)

15:20 (KST) → engine.GenerateDailyReport(ctx)
            → db.SaveReport(date, report)

16:00 (KST) → wsClient.Disconnect()
```

---

## 트레이딩 엔진 사이클 (09:15 ~ 15:15 반복)

```
현재 포지션 수 < max_positions?
  YES →
    1. 당일 거래 종목 코드 제외 목록 조회 (DB)
    2. 설정된 순위 API 호출 (volume/strength/exec_count/disparity)
    3. GetInquireBalance() → 가용자금 확인
    4. ClaudeClient.SelectStock(rankings, cash, excluded) → stockCode, reason
    5. CheckOrderFeasibility(stockCode) → orderableQty
    6. PlaceOrder(시장가, qty * order_amount_pct%)
    7. ExecCh drain-and-match (KISOrderID 매칭, 최대 5분)
       └─ 타임아웃 → CancelOrder() → 처음으로
    8. Monitor.Register(pos, SoldCh=engine.soldCh)
    9. 다시 1로 (max_positions 미충족 시 즉시 다음 종목)

  NO → soldCh 대기 (or 30초 주기 re-check)
       └─ sold 수신 → 처음으로
```

---

## 실시간 가격 모니터링 데이터 플로우

```
KIS WebSocket (H0STCNT0)
  ↓ PriceCh (buffered, 256)
monitor.StartPriceConsumer()
  ↓ HandlePrice(isTest=false)
  ├─ price ≥ TargetPrice → executeSell() → KIS 시장가 매도
  │                      → SoldCh <- stockCode (엔진 신호)
  │                      → mqtt.PublishAlert(TARGET_HIT)
  │                      → Remove()
  └─ price ≤ StopPrice  → executeSell() → KIS 시장가 매도
                         → SoldCh <- stockCode (엔진 신호)
                         → mqtt.PublishAlert(STOP_HIT)
                         → Remove()

KIS WebSocket (H0STCNI0 체결통보)
  ↓ ExecCh (buffered, 64)
trader.Engine.waitForFill() — KISOrderID 매칭, 5분 타임아웃

monitor.StartIndicatorChecker() (5분 주기, 별도 goroutine)
  → GetStockInfo(code) → RSI14, MACDLine, MACDSignal
  → rsi_overbought 조건 or macd_bearish 조건 충족 시
     → executeSell() → SoldCh <- stockCode → Remove()

15:15 LiquidateAll():
  → GetHoldings → PlaceSellOrder(시장가) → mqtt.PublishAlert(LIQUIDATION)
```

---

## API Endpoint Map

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/balance` | `GetBalance` | 계좌 잔고 조회 |
| GET | `/api/positions` | `GetPositions` | 실시간 보유 종목 |
| GET | `/api/stock/:code` | `GetStock` | 현재가 + MA5/MA20 + RSI14 + MACD |
| GET | `/api/stock/:code/chart` | `GetStockChart` | 캔들 차트 (1m/5m/1h) |
| GET | `/api/orders` | `GetOrders` | 주문 내역 (`?sync=true` KIS 동기화) |
| POST | `/api/orders` | `PlaceOrder` | 수동 주문 실행 |
| POST | `/api/orders/:id/cancel` | `CancelOrder` | KIS 미체결 주문 취소 |
| DELETE | `/api/orders/:id` | `DeleteOrder` | 주문 단건 삭제 |
| GET | `/api/orders/feasibility` | `GetFeasibility` | 주문가능수량/금액 조회 |
| GET | `/api/server/status` | `GetServerStatus` | 서버 통합 상태 (시장·WebSocket·모니터·trader_state) |
| GET | `/api/monitor/positions` | `GetMonitorPositions` | 모니터링 중인 포지션 목록 |
| DELETE | `/api/monitor/positions/:code` | `RemoveMonitorPosition` | 모니터링 포지션 제거 |
| GET | `/api/market/status` | `GetMarketStatus` | 장운영 여부 |
| GET | `/api/ranking/volume` | `GetVolumeRank` | 거래량 순위 |
| GET | `/api/ranking/strength` | `GetStrengthRank` | 체결강도 순위 |
| GET | `/api/ranking/exec-count` | `GetExecCountRank` | 대량체결건수 순위 |
| GET | `/api/ranking/disparity` | `GetDisparityRank` | 이격도 순위 |
| GET | `/api/logs/kis` | `GetKISLogs` | KIS API 에러 로그 |
| DELETE | `/api/logs/kis/:id` | `DeleteKISLog` | 에러 로그 단건 삭제 |
| GET | `/api/settings` | `GetSettings` | 모든 설정 조회 |
| PATCH | `/api/settings` | `UpdateSettings` | 설정 변경 |
| GET | `/api/reports` | `GetReports` | 리포트 날짜 목록 |
| GET | `/api/reports/:date` | `GetReport` | 특정 날짜 리포트 전문 |
| GET | `/api/debug/balance` | `DebugRawBalance` | KIS 잔고 원본 응답 |
| POST | `/api/debug/ws` | `DebugWSConnect` | WebSocket 수동 연결 |
| DELETE | `/api/debug/ws` | `DebugWSDisconnect` | WebSocket 수동 해제 |
| POST | `/api/debug/price` | `DebugInjectPrice` | 가짜 가격 이벤트 주입 (is_test=true) |
| POST | `/api/debug/monitor` | `DebugRegisterMonitor` | 모니터 포지션 직접 등록 (KIS 주문 없이) |
| POST | `/api/debug/liquidate` | `DebugLiquidate` | LiquidateAll 수동 트리거 |
| GET | `/health` | (inline) | 헬스 체크 |

---

## MQTT 토픽 맵

| 토픽 | 이벤트 | 발행 조건 |
|------|--------|----------|
| `trading/alert/{stock_code}` | `TARGET_HIT` | 현재가 ≥ 목표가 → 자동 매도 완료 |
| `trading/alert/{stock_code}` | `STOP_HIT` | 현재가 ≤ 손절가 → 자동 매도 완료 |
| `trading/liquidation` | `LIQUIDATION` | 15:15 전량 청산 |
