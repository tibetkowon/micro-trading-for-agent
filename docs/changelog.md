# Changelog

## 2026-03-06 — 순위별 필터 설정 (거래량증가율/체결강도/순매수/이격도)

- **database/db.go**: `TradingSettings`에 순위 필터 필드 5개 추가, 기본값 자동 삽입
- **trader/engine.go**: `getRankings()`에 필터 로직 추가, `RankItem`에 지표 필드 전달
- **trader/claude.go**: `RankItem`에 `VolIncrRate`, `Strength`, `NetBuyQty`, `DisparityD20` 필드 추가
- **api/handlers.go**: `GetSettings`/`UpdateSettings` 신규 키 처리
- **frontend/Settings.jsx**: 순위 유형별 체크박스 아래 필터 입력창 표시

## 2026-03-06 — 매도 조건 우선순위 UI + CLAUDE.md 스킬 체크리스트 강화

- **frontend/Settings.jsx**: 매도 조건 체크박스 → 순서 변경 가능한 우선순위 리스트로 교체 (▲▼ 버튼, ＋/✕ 토글)
- **CLAUDE.md**: 스킬 트리거 조건 강화 — MANDATORY POST-TASK CHECKLIST 테이블 추가

## 2026-03-06 — 자율 트레이딩 엔진 (Claude API 기반) 도입

- **trader/claude.go** (신규): Claude API 클라이언트 — `SelectStock` (JSON 응답 파싱), `GenerateReport` (한국어 마크다운 일일 리포트)
- **trader/engine.go** (신규): 자율 트레이딩 엔진 — IDLE/SELECTING/ORDERING/WAITING_FILL/MONITORING 상태 머신, ExecCh 체결 대기(5분 타임아웃), 미체결 시 취소 후 재선정
- **database/db.go**: `reports` 테이블 추가, `TradingSettings` 구조체 + `GetTradingSettings()` 헬퍼, 신규 설정 키 12개 기본값 자동 삽입 (`INSERT OR IGNORE`)
- **models/models.go**: `Report` 구조체 추가
- **config/config.go**: `AnthropicAPIKey` 필드 추가 (`ANTHROPIC_API_KEY` 환경변수)
- **monitor/monitor.go**: `MonitoredEntry.SoldCh chan<- string` 추가 (매도 완료 시 엔진 신호), `StartIndicatorChecker()` 추가 (RSI과매수/MACD데드크로스 주기적 평가)
- **api/handlers.go**: `Handler.engine` 필드 + `SetEngine()`, `GetServerStatus`에 `trader_state` 추가, `GetSettings`/`UpdateSettings` 신규 설정 키 처리, `GetReports`/`GetReport` 핸들러 추가
- **api/router.go**: `/api/reports`, `/api/reports/:date` 라우트 추가
- **main.go**: `ClaudeClient` 초기화, `Engine` 생성 및 `handler.SetEngine` 주입, `runMarketScheduler` 확장 (09:00 거래 가능 확인, 09:15 엔진 시작 + 지표체커 시작, 15:15 엔진 중지, 15:20 일일 리포트 생성/저장)
- **frontend**: `Reports.jsx` 신규, `Settings.jsx` 신규 설정 섹션 6개 추가, `Dashboard.jsx` 트레이더 상태 표시, `Orders.jsx` 날짜 드롭다운 제거, `App.jsx` 리포트 라우트 추가

## 2026-03-05 — 목표/손절가 도달 시 자동 매도 + MQTT 페이로드 개선

- **monitor/monitor.go**: `HandlePrice` — 목표/손절 도달 시 KIS 시장가 매도(`executeSell`) 후 MQTT 발행. `isTest=true`면 매도 스킵 (테스트 전용)
- **monitor/monitor.go**: `executeSell()` 신규 — GetHoldings → PlaceSellOrder(시장가) → sellQty 반환
- **monitor/monitor.go**: `LiquidateAll` — 청산 시 현재가(근사 매도가) + 매도수량을 MQTT 페이로드에 포함
- **mqtt/publisher.go**: `AlertPayload`에 `sell_qty`, `profit_amount` 필드 추가; `PublishAlert` 시그니처에 `sellQty int` 파라미터 추가

## 2026-03-05 — Debug API + UI (장 외 테스트 기능)

- **mqtt/publisher.go**: `AlertPayload`에 `IsTest bool` (`is_test`) 필드 추가; `PublishAlert` 시그니처에 `isTest bool` 파라미터 추가
- **monitor/monitor.go**: `HandlePrice(stockCode, price, isTest bool)` 시그니처 변경; `PublishAlert` 3곳 — 정상 호출 `false`, Debug 주입 `true` 전달
- **api/handlers.go**: Debug 핸들러 5종 추가 — `DebugWSConnect`, `DebugWSDisconnect`, `DebugInjectPrice`, `DebugRegisterMonitor`, `DebugLiquidate`
- **api/router.go**: `/api/debug/*` 라우트 그룹으로 정리 (기존 `GET /balance` 포함 6종)
- **frontend/src/pages/Debug.jsx** (신규): WebSocket 제어, 포지션 등록, 가격 주입, LiquidateAll UI + 응답 로그 패널
- **frontend/src/App.jsx**: `/debug` 라우트 및 네비게이션 "디버그" 항목 추가

## 2026-03-04 — Phase 1: KIS WebSocket + 실시간 모니터링 + MQTT

- **kis/websocket.go** (신규): `WebSocketClient` — KIS WebSocket 연결/구독/재연결, AES-256-CBC 복호화, `PriceCh`/`ExecCh` 채널
- **mqtt/publisher.go** (신규): `Publisher` — Paho MQTT 클라이언트, `PublishAlert()` 포함, 브로커 미연결 시 graceful 처리
- **monitor/monitor.go** (신규): `Monitor` — 목표가/손절가 실시간 체크, DB 영속화, 서버 재시작 복구, `LiquidateAll()` (15:15)
- **kis/client.go**: `GetApprovalKey()` 추가 — WebSocket approval_key 발급 (`POST /oauth2/Approval`)
- **config/config.go**: `KISHTSID`, `MQTTBrokerURL`, `MQTTClientID` 환경변수 추가
- **models/models.go**: `Order`에 `TargetPct`/`StopPct` 추가, `MonitoredPosition` 구조체 신규
- **database/db.go**: `monitored_positions` 테이블, `orders.target_pct`/`stop_pct` 컬럼 마이그레이션 추가
- **agent/order.go**: `PlaceOrderRequest`에 `TargetPct`/`StopPct` 추가, INSERT에 포함
- **api/handlers.go**: `PlaceOrder` target/stop 수신 → 모니터 등록, `GetServerStatus`/`GetMonitorPositions`/`RemoveMonitorPosition` 신규
- **api/router.go**: `/api/server/status`, `/api/monitor/positions`, `/api/monitor/positions/:code` 라우트 등록
- **main.go**: MQTT/WebSocket/Monitor 초기화, 장운영 스케줄러(08:50/15:15/16:00), 폴링 주기 3분→5분 조정

## 2026-03-03 — KIS inquire-daily-ccld API 파라미터 버그 수정

- **kis/client.go**: `GetOrderHistory()` 쿼리에 누락된 필수 파라미터 3개 추가 (`INQR_DVSN_1`, `INQR_DVSN_3`, `EXCG_ID_DVSN_CD=ALL`)
- **kis/client.go**: 스펙에 없는 `CANC_YN` 파라미터 제거 — API 게이트웨이가 HTML 오류 반환하던 원인
- **kis/client.go**: `get()` 헬퍼에 `Content-Type: application/json; charset=utf-8` 헤더 추가 (GET 요청에도 명세 required)
- **영향**: 체결 완료 거래가 대기(PENDING) 상태로 표시되던 현상 해결

## 2026-03-02 — 장운영일 체크 + Order Sync 스케줄러 최적화

- **KIS client**: `HolidayInfo` DTO 및 `GetMarketHolidayInfo()` 메서드 추가 (`CTCA0903R`)
- **agent/market.go** (신규): `IsMarketOpen()` 함수 — KST 평일·장 운영 시간·KIS 영업일 3중 체크, 당일 메모리 캐시로 KIS 호출 하루 1회 제한
- **agent/history.go**: `StartOrderSyncScheduler` ticker 블록에 `IsMarketOpen()` 가드 추가 — 장 마감·공휴일 시 sync skip
- **api/handlers.go**: `GET /api/market/status` 핸들러 추가 (reason 필드: open/weekend/outside_hours/holiday/check_failed)
- **api/router.go**: `/api/market/status` 라우트 등록
- **main.go**: `import _ "time/tzdata"` 추가 — NCP Micro tzdata 미설치 환경 대비
