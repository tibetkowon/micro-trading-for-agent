# Project Architecture

> Last updated: 2026-03-04 (rev 5 — Phase 1: WebSocket + Monitor + MQTT)

## Directory Tree

```
micro-trading-for-agent/
├── .github/
│   └── workflows/
│       └── ci.yml              # CI: Go build/test/fmt + React lint/build; CD: linux/amd64 cross-compile → SCP → rsync → systemctl restart
├── .claude/
│   └── skills/                 # AI 에이전트 행동 지침 파일 (.md)
├── backend/                    # Go backend root
│   ├── cmd/
│   │   └── server/
│   │       └── main.go         # 진입점; MQTT·WebSocket·Monitor 초기화, 장운영 스케줄러, 서버 시작/종료
│   ├── internal/
│   │   ├── config/
│   │   │   └── config.go       # .env 로드 (godotenv); KIS·MQTT·서버 설정 포함
│   │   ├── database/
│   │   │   └── db.go           # SQLite 초기화 + 스키마 마이그레이션 (자동 실행)
│   │   ├── models/
│   │   │   └── models.go       # DB 테이블과 1:1 매핑되는 Go 구조체 + 상수
│   │   ├── logger/
│   │   │   └── logger.go       # 구조화 JSON 로깅; KISError()는 필수 필드(error_code, timestamp, raw_response) 강제
│   │   ├── kis/
│   │   │   ├── client.go       # KIS REST API 클라이언트; GetApprovalKey() 포함
│   │   │   ├── websocket.go    # KIS WebSocket 클라이언트 (gorilla/websocket, AES-256-CBC 복호화)
│   │   │   ├── chart.go        # KIS 차트 API: GetMinuteChart(1분봉), GetDailyChart(일봉)
│   │   │   ├── token.go        # OAuth 토큰 발급 + 20시간 자동 갱신 + 자격증명 지문 체크
│   │   │   └── ratelimiter.go  # TPS 리미터 (15 req/s, golang.org/x/time/rate)
│   │   ├── mqtt/
│   │   │   └── publisher.go    # Paho MQTT 퍼블리셔; PublishAlert() (목표가/손절가/청산/리포트)
│   │   ├── monitor/
│   │   │   └── monitor.go      # 포지션 모니터; 목표가/손절가 실시간 체크, DB 영속화, LiquidateAll()
│   │   ├── agent/
│   │   │   ├── market.go       # IsMarketOpen(): KST 평일·장 시간·KIS 영업일 3중 체크 + 당일 캐시
│   │   │   ├── stock_info.go   # GetStockInfo: 현재가 + MA5/MA20 (일봉 차트 기반)
│   │   │   ├── chart.go        # GetChart: OHLCV 캔들 (1m/5m/1h), 페이지네이션 + 집계
│   │   │   ├── balance.go      # 계좌 잔고 조회 + DB 스냅샷 저장
│   │   │   ├── order.go        # 매수/매도 주문 실행; target_pct/stop_pct 포함
│   │   │   ├── ranking.go      # 거래량/체결강도/대량체결건수/이격도 순위 조회
│   │   │   └── history.go      # KIS 체결 내역 동기화; StartOrderSyncScheduler (5분 ticker, 장중에만 실행)
│   │   └── api/
│   │       ├── handlers.go     # HTTP 핸들러: 잔고/종목/차트/주문/모니터/서버상태/로그/순위/설정
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
│   │       ├── Dashboard.jsx   # 잔고 / 수익률 카드
│   │       ├── Orders.jsx      # 주문 내역 테이블
│   │       ├── KISLogs.jsx     # KIS API 에러 로그 뷰어
│   │       └── Settings.jsx    # 계정 정보 (읽기 전용); .env 기반 설정 확인
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
│   │   └── mqtt-setup.md       # MQTT 브로커 설치 및 에이전트 연동 가이드
│   ├── kis-api/                # KIS API 공식 명세서 (기본시세/순위분석/종목정보/주문계좌/인증/실시간)
│   ├── plans/                  # 기능 구현 계획 문서
│   └── reviews/                # 한국어 코드 리뷰 문서
├── SKILL.md                    # 에이전트 스킬 퀵 레퍼런스
├── .env.example                # 환경변수 템플릿 (시크릿 미포함)
├── .gitignore                  # .env, *.db, node_modules, 바이너리 제외
├── CLAUDE.md                   # AI 에이전트 프로젝트 지침
└── README.md                   # 프로젝트 개요
```

## Component Responsibilities

### `backend/internal/config`
- **Role:** 환경변수에서 모든 설정을 로드 (하드코딩 금지).
- **Put here:** 환경 키 파싱, 기본값, 파생 헬퍼.
- **Do NOT put here:** 비즈니스 로직, DB 쿼리, HTTP 핸들러.
- **현재 관리 항목:** KIS 자격증명, MQTT 브로커, HTS ID, 서버 포트, DB 경로

### `backend/internal/database`
- **Role:** SQLite 연결 초기화 및 서버 시작 시 스키마 마이그레이션 자동 실행.
- **Put here:** `sql.DB` 래퍼, 마이그레이션 SQL, 커넥션 풀 설정.
- **Do NOT put here:** 비즈니스 로직, 비즈니스 코드가 사용하는 쿼리.

### `backend/internal/models`
- **Role:** DB 테이블과 1:1 대응하는 공유 데이터 구조체.
- **Put here:** 순수 Go 구조체, 열거형 상수 (`OrderType`, `OrderStatus`, `OrderSource`).
- **Do NOT put here:** 비즈니스 로직이 있는 메서드, DB 상호작용.

### `backend/internal/logger`
- **Role:** 구조화 JSON 로깅. `KISError()`는 CLAUDE.md 필수 필드(error_code, timestamp, raw_response)를 강제.

### `backend/internal/kis`
- **Role:** KIS API HTTP/WebSocket 통신 레이어.
- **`client.go`** — REST API 요청/응답, 토큰 주입, Rate Limiting, `GetApprovalKey()`
- **`websocket.go`** — WebSocket 연결/재연결, `H0STCNT0`(체결가)/`H0STCNI0`(체결통보) 구독, AES-256-CBC 복호화, `PriceCh`/`ExecCh` 이벤트 채널
- **`token.go`** — OAuth 토큰 발급·갱신·캐시, 자격증명 지문 체크
- **`ratelimiter.go`** — 15 req/s TPS 제한

### `backend/internal/mqtt`
- **Role:** MQTT 메시지 발행 전담 레이어.
- **`publisher.go`** — Paho MQTT 클라이언트 래퍼; `PublishAlert()` — 목표가/손절가/청산/리포트 알림 발행
- 브로커 미연결 시 에러 로그만 남기고 서버 기동은 정상 유지

### `backend/internal/monitor`
- **Role:** 보유 포지션 실시간 모니터링 (목표가/손절가 비교).
- **`monitor.go`** — `Register()`/`Remove()` 포지션 관리, `HandlePrice()` 가격 이벤트 처리, `LiquidateAll()` 장마감 전량 청산, DB 영속화 + 서버 재시작 복구

### `backend/internal/agent`
- **Role:** AI 에이전트 액션 함수. KIS API 데이터와 DB 영속성을 연결하는 거래 루프의 핵심.
- `market.go` — `IsMarketOpen()`: KST 평일·장 운영 시간(9:00~15:30)·KIS 영업일 여부를 체크. 당일 1회 캐시.
- `history.go` — `StartOrderSyncScheduler()`: 5분 간격, **장 중에만** KIS 체결 내역 동기화.
- `order.go` — 주문 실행; `TargetPct`/`StopPct` 파라미터로 모니터 등록 트리거.

### `backend/internal/api`
- **Role:** HTTP 레이어. 입력 검증 → agent/DB 함수 호출 → JSON 응답의 얇은 핸들러.

## 장운영 스케줄러 (main.go)

```
08:50 (KST) → GetApprovalKey() → ws.StartWithReconnect() → SubscribeExecNotice()
15:15 (KST) → monitor.LiquidateAll() → 시장가 전량 매도
16:00 (KST) → ws.Disconnect()
```

## 실시간 가격 모니터링 데이터 플로우

```
KIS WebSocket (H0STCNT0)
  ↓ PriceCh (buffered channel, 256)
monitor.StartPriceConsumer()
  ↓ HandlePrice()
  ├─ price ≥ TargetPrice → mqtt.PublishAlert(TARGET_HIT) → monitor.Remove()
  └─ price ≤ StopPrice  → mqtt.PublishAlert(STOP_HIT)  → monitor.Remove()
```

## API Endpoint Map

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/balance` | `GetBalance` | 계좌 잔고 조회 |
| GET | `/api/positions` | `GetPositions` | 실시간 보유 종목 |
| GET | `/api/stock/:code` | `GetStock` | 현재가 + MA5/MA20 |
| GET | `/api/stock/:code/chart` | `GetStockChart` | 캔들 차트 (1m/5m/1h) |
| GET | `/api/orders` | `GetOrders` | 주문 내역 (`?sync=true` KIS 동기화) |
| POST | `/api/orders` | `PlaceOrder` | 주문 실행 + 모니터 등록 (target_pct/stop_pct) |
| POST | `/api/orders/:id/cancel` | `CancelOrder` | KIS 미체결 주문 취소 |
| DELETE | `/api/orders/:id` | `DeleteOrder` | 주문 단건 삭제 |
| GET | `/api/orders/feasibility` | `GetFeasibility` | 주문가능수량/금액 조회 |
| GET | `/api/server/status` | `GetServerStatus` | 서버 통합 상태 (시장·WebSocket·모니터) |
| GET | `/api/monitor/positions` | `GetMonitorPositions` | 모니터링 중인 포지션 목록 |
| DELETE | `/api/monitor/positions/:code` | `RemoveMonitorPosition` | 모니터링 포지션 제거 |
| GET | `/api/market/status` | `GetMarketStatus` | 장운영 여부 (open/weekend/outside_hours/holiday/check_failed) |
| GET | `/api/ranking/volume` | `GetVolumeRank` | 거래량 순위 |
| GET | `/api/ranking/strength` | `GetStrengthRank` | 체결강도 순위 |
| GET | `/api/ranking/exec-count` | `GetExecCountRank` | 대량체결건수 순위 |
| GET | `/api/ranking/disparity` | `GetDisparityRank` | 이격도 순위 |
| GET | `/api/logs/kis` | `GetKISLogs` | KIS API 에러 로그 |
| DELETE | `/api/logs/kis/:id` | `DeleteKISLog` | 에러 로그 단건 삭제 |
| GET | `/api/settings` | `GetSettings` | 설정 조회 (마스킹) |
| GET | `/api/debug/balance` | `DebugRawBalance` | KIS 잔고 원본 응답 |
| GET | `/health` | (inline) | 헬스 체크 |

## MQTT 토픽 맵

| 토픽 | 발행 조건 |
|------|----------|
| `trading/alert/{stock_code}` | 목표가 도달 (`TARGET_HIT`) 또는 손절가 도달 (`STOP_HIT`) |
| `trading/liquidation` | 15:15 장마감 전량 청산 (`LIQUIDATION`) |
| `trading/report` | 일일 거래 리포트 (`DAILY_REPORT`) |
