# Changelog

> Entries are in reverse chronological order (newest first).

---

## 2026-02-27 (2)

### [Feature] 수동 거래 자동 임포트 + 주문내역 정렬 개선

**Description:**
KIS 동기화 시 에이전트가 생성하지 않은 주문(사용자가 KIS 앱/웹에서 직접 체결한 수동 거래)을 자동 감지하여 `orders` 테이블에 임포트합니다. 이로써 에이전트가 전체 거래 내역을 파악하여 오판을 방지할 수 있습니다.

**동작 방식:**
1. `GetOrderHistory()` 동기화 시 각 KIS 주문의 `odno`를 로컬 DB와 대조
2. 로컬 DB에 없는 주문 → `source = 'MANUAL'`로 자동 INSERT
3. 이미 있는 주문 → 기존 UPDATE 로직 그대로 (상태/체결가/종목명 갱신)
4. 중복 삽입 방지: 동일 `odno`가 이미 존재하면 INSERT하지 않음

**수동 거래 임포트 시 파싱 필드:**
- `pdno` → `stock_code`, `prdt_name` → `stock_name`
- `sll_buy_dvsn_cd` (01=SELL, 02=BUY) → `order_type`
- `ord_unpr` → `price`, `avg_prvs` → `filled_price`
- `ord_qty` → `qty`, `cncl_yn` + `tot_ccld_qty` → `status`
- `ord_dt` + `ord_tmd` → `created_at` (KIS 실제 주문 시각)

**정렬 개선:**
- 기존: `ORDER BY id DESC` — 수동 거래가 늦게 임포트되어 순서 뒤틀림
- 변경: `ORDER BY created_at DESC, id DESC` — 실제 주문 시각 기준 정렬

**백그라운드 스케줄러 변경:**
- 기존: PENDING/PARTIALLY_FILLED 주문이 없으면 KIS API 호출 생략
- 변경: 항상 동기화 실행 (수동 거래 감지를 위해)

**UI:**
- 주문내역 테이블에 "구분" 열 추가
  - `MANUAL` → 주황색 "수동" 배지
  - `AGENT` → 회색 "에이전트" 배지

**Files Touched:**
- `backend/internal/database/db.go` — `source TEXT DEFAULT 'AGENT'` ALTER TABLE 마이그레이션 추가
- `backend/internal/models/models.go` — `OrderSource` 타입, `AGENT`/`MANUAL` 상수, `Order.Source` 필드 추가
- `backend/internal/agent/history.go` — 수동 거래 INSERT 로직, 정렬 수정, `source` 컬럼 SELECT/Scan 추가, 스케줄러 항상 실행
- `frontend/src/pages/Orders.jsx` — "구분" 열 + 수동/에이전트 배지

**Pending/Next Steps:**
- KIS는 당일 주문만 반환하므로 이전 날의 수동 거래는 임포트되지 않음 (향후 날짜 범위 조회 확장 검토)

---

## 2026-02-27

### [Feature] 기술적 지표 추가 — 거래대금, RSI(14), MACD(12,26,9)

**Description:**
에이전트가 종목 진입 판단 시 사용할 수 있도록 `GET /api/stock/:code` 응답에 3가지 지표를 추가했습니다.

**추가된 응답 필드:**
- `trading_value`: 거래대금(current_price × volume, KRW). 거래량보다 실질 자금 유입을 직관적으로 반영
- `rsi14`: 5분봉 기준 RSI(14) — Wilder's smoothing 방식. 0 = 데이터 부족
- `macd_line / macd_signal / macd_histogram`: 5분봉 기준 MACD(12,26,9). 0 = 데이터 부족

**구현 방식:**
- 5분봉 데이터: 1분봉 200개 fetch → 5분봉 집계 (~40봉). MACD 최소 요구 34봉 충족
- RSI: Wilder's smoothing (기본 SMA 시드 → 이후 스무딩 평균)
- MACD: calcEMA(closes, period) 슬라이스 기반 → MACD series → Signal EMA(9)
- 장 시작 직후 / 거래 정지 종목은 0 반환 (에이전트가 해당 지표 판단 보류)

**Files Touched:**
- `backend/internal/agent/stock_info.go` — StockInfo 구조체 확장 + calcRSI/calcEMA/calcMACD 헬퍼 추가 + GetStockInfo에 5분봉 fetch 및 지표 계산 로직

**Pending/Next Steps:**
- 없음

---

### [Feature] 주문취소 API 추가 — POST /api/orders/:id/cancel

**Description:**
에이전트가 미체결 주문을 KIS에 실제 취소 요청할 수 있는 엔드포인트를 추가했습니다. 기존 `DELETE /api/orders/:id`는 로컬 DB 레코드만 삭제하고 KIS에 취소 요청을 하지 않던 문제를 해결했습니다.

**동작 흐름:**
1. 로컬 DB에서 kis_order_id 및 현재 status 조회
2. TTTC0084R(주식정정취소가능주문조회) 호출 → KIS 취소 가능 목록에서 해당 주문 확인 + psbl_qty > 0 검증
3. TTTC0013U(주식주문 정정취소, RVSE_CNCL_DVSN_CD=02) 호출 → 잔량 전부(QTY_ALL_ORD_YN=Y) 취소
4. 로컬 DB status → CANCELLED 업데이트

**오류 케이스:**
- 이미 FILLED된 주문 → 400 오류
- KIS 취소 가능 목록에 없는 주문(이미 체결/결제) → 오류
- psbl_qty == 0 → 오류

**Files Touched:**
- `backend/internal/kis/client.go` — CancellableOrderItem DTO, GetCancellableOrders(TTTC0084R), CancelKISOrder(TTTC0013U) 추가
- `backend/internal/agent/order.go` — CancelOrderResult + CancelOrder() 함수 추가
- `backend/internal/api/handlers.go` — CancelOrder 핸들러 추가
- `backend/internal/api/router.go` — POST /api/orders/:id/cancel 라우트 추가
- `SKILL.md`, `docs/agent-skill.md` — 에이전트 가이드 전면 개편 (워크플로우, 지표 해석, 주문 상태 레퍼런스 추가)

**Pending/Next Steps:**
- 실제 KIS 환경에서 TTTC0084R / TTTC0013U 호출 확인
- PARTIALLY_FILLED 주문 취소 시 psbl_qty가 잔여 수량을 정확히 반영하는지 확인

---

## 2026-02-26 (2)

### [Feature] 순위 API ETF/비정상종목 제외 및 가격 필터 추가

**Description:**
4개 순위 API(거래량/체결강도/대량체결건수/이격도) 모두에 ETF 자동 제거 및 가격 범위 필터를 추가했습니다.

**ETF 제외:**
- `FID_TRGT_EXLS_CLS_CODE=1111111111` (10자리) 적용 → 투자위험/경고/주의/관리종목/정리매매/불성실공시/우선주/거래정지/ETF/ETN/신용주문불가/SPAC 모두 제외
- 결과: 일반 보통주만 반환 (하드코딩, 별도 파라미터 없음)
- 거래량순위(FHPST01710000)는 KIS 문서에 비트마스크 명시; 나머지 3개는 동일 값 적용 후 실제 API 테스트로 동작 확인 필요

**가격 필터:**
- `price_min` / `price_max` query param — 직접 가격 범위 입력 (원, 빈값=전체)
- `use_balance_filter=true` — 잔액 API(TTTC8434R)의 `dnca_tot_amt`(예수금)를 price_max로 자동 설정; 예수금 0이거나 조회 실패 시 필터 미적용(fallback)

**사용 예시:**
```
GET /api/ranking/volume?price_max=50000
GET /api/ranking/volume?use_balance_filter=true
GET /api/ranking/strength?use_balance_filter=true
GET /api/ranking/disparity?price_min=10000&price_max=100000
```

**Files Touched:**
- `backend/internal/kis/client.go` — 4개 ranking 함수 ETF 제외값 + priceMin/priceMax 파라미터 추가
- `backend/internal/agent/ranking.go` — 함수 시그니처에 priceMin/priceMax 추가
- `backend/internal/api/handlers.go` — `resolvePriceFilter()` 헬퍼 + 4개 핸들러 price_min/price_max/use_balance_filter 처리

**Pending/Next Steps:**
- 실제 KIS API 호출 후 체결강도/대량체결건수/이격도 순위에서 ETF가 실제로 제외되는지 확인
- 체결강도 API(`fid_trgt_exls_cls_code`) 비트마스크 미지원 시 응답 후처리 필터 추가 검토

---

## 2026-02-26

### [Fix] KIS 토큰 자동 갱신 불동작 3가지 버그 수정 (EGW00123)

**Description:**
`{"rt_cd":"1","msg_cd":"EGW00123","msg1":"기간이 만료된 token 입니다."}` 오류가 지속 발생하던 문제를 근본 원인부터 수정했습니다.

**원인 1 — StartAutoRefresh 타이머 리셋 버그 (`token.go`)**
- 기존: `time.NewTicker(20h)` 가 항상 서버 부팅 시점부터 카운트
- 시나리오: 토큰 발급 18h 후 서버 재시작 → 타이머가 20h 리셋 → 토큰 만료(T+24h) 후 타이머 발동(T+38h) 사이 **14시간 공백** 발생
- 수정: 서버 시작 시 DB 최신 토큰의 `issued_at` 을 읽어 `(20h - 경과시간)` 을 첫 delay로 계산. 이미 지났으면 즉시 재발급.

**원인 2 — API 호출 직전 만료 검사 없음 (`client.go`)**
- 기존: `GetCurrentToken()` → 만료 여부 무시하고 DB 최신 토큰 반환
- 수정: `EnsureToken()` 으로 교체 → "잔여 1시간 미만이면 재발급" 로직이 API 호출 직전마다 실행됨
- 적용 위치: `get()` (L254), `placeOrder()` (L284) 양쪽 모두

**원인 3 — KIS GET 응답의 rt_cd 에러 미감지 (`client.go`)**
- 기존: HTTP 200이면 무조건 성공으로 처리. KIS는 토큰 만료를 HTTP 200 + `rt_cd:"1"` 로 반환하므로 에러가 무시됨
- 수정: HTTP 200 이후 응답 본문에서 `rt_cd` 파싱 추가. `rt_cd=="1"` 이면 `logAPIError()` 호출 및 에러 반환. `EGW00123` 인 경우 즉시 `IssueToken()` 트리거 (안전망)

**Files Touched:**
- `backend/internal/kis/token.go` — `StartAutoRefresh` 타이머 로직 재작성
- `backend/internal/kis/client.go` — `EnsureToken` 교체, `rt_cd` 응답 본문 검사 추가

**Pending/Next Steps:**
- KIS 토큰 만료 에러 발생 후 다음 요청이 새 토큰으로 성공하는지 실제 환경에서 확인

---

## 2026-02-25 (4)

### [Refactor] 대시보드/잔고 API TTTC8434R 단일 호출로 최적화 + 에이전트 주문 흐름 개선

**Description:**
잔고 대시보드를 TTTC8434R(주식잔고조회) 단일 API 호출로 단순화하고, 에이전트 주문 흐름에 TTTC8908R(매수가능조회) 종목별 수량 확인 단계를 추가했습니다.

**배경:**
- TTTC8434R의 `asst_icdc_erng_rt`(자산증감수익률)는 KIS가 "데이터 미제공" 처리 → 백엔드에서 직접 계산
- TTTC8908R은 특정 종목·가격·수수료를 반영한 정확한 주문가능수량을 반환하므로 에이전트 주문 직전에 호출이 적절

**[Refactor] backend/internal/kis/client.go**
- `InquireBalanceOutput2`: `asst_icdc_amt`(자산증감액), `bfdy_tot_asst_evlu_amt`(전일총자산평가금액) 추가; 매입금액/평가손익 제거
- `AvailableOrderResponse`: `ord_psbl_qty`(주문가능수량) + `ord_psbl_cash`(주문가능현금)으로 재설계
- `GetAvailableOrder(ctx, stockCode string)`: 종목코드 파라미터 추가 (에이전트 주문 전 종목별 호출 목적)

**[Refactor] backend/internal/agent/balance.go**
- `AccountBalance` 구조체: `tradable_amount` 제거, `asset_change_amt` + `asset_change_rate` 추가
- `GetAccountBalance`: TTTC8434R 단일 호출로 대시보드 데이터 완성; `GetAvailableOrder` 호출 제거
- 자산증감수익률 계산: `asst_icdc_amt / bfdy_tot_asst_evlu_amt × 100`

**[Feature] backend/internal/agent/order.go**
- `CheckOrderFeasibility(ctx, client, stockCode)` 추가
  - TTTC8908R 호출 → `OrderableQty`(수량) + `AvailableCash`(재선정 기준 현금) 반환
  - `qty > 0` 이면 주문 / `qty == 0` 이면 `AvailableCash` 기준으로 종목 재선정

**[Refactor] frontend/src/pages/Dashboard.jsx**
- 카드 5개 → 4개로 재편: 총평가금액 / 출금가능금액 / 자산증감액 / 자산증감수익률
- 거래가능금액(주문가능금액) 카드 제거

**Files Touched:**
- `backend/internal/kis/client.go`
- `backend/internal/agent/balance.go`
- `backend/internal/agent/order.go`
- `frontend/src/pages/Dashboard.jsx`

**Pending/Next Steps:**
- 에이전트 종목 선정 로직 구현 시 `CheckOrderFeasibility` 연동 및 재선정 루프 구현

---

## 2026-02-25 (3)

### [Feature] 주문/로그 개별 삭제 + KIS 체결 자동 동기화

**Description:**
주문 내역과 KIS 에러 로그에 개별 삭제 기능을 추가하고, 주문 체결 상태가 자동으로 갱신되지 않던 버그를 해결했습니다.

**[Feature] 개별 삭제 API**
- `DELETE /api/orders/:id` — orders 테이블 단건 삭제
- `DELETE /api/logs/kis/:id` — kis_api_logs 테이블 단건 삭제

**[Feature] KIS 체결 동기화**
- `GET /api/orders?sync=true` — KIS API에서 최신 체결내역 조회 후 DB 갱신, 이후 목록 반환
  - PENDING → FILLED / PARTIALLY_FILLED 상태 자동 업데이트
  - filled_price (평균체결가) 갱신

**[Feature] 3분 주기 백그라운드 스케줄러**
- `agent.StartOrderSyncScheduler(ctx, client, db, 3*time.Minute)` — Go `time.Ticker` 기반 고루틴
- PENDING/PARTIALLY_FILLED 주문이 없으면 KIS API 호출 생략 (TPS 절약)
- 서버 `ctx` 취소 시 graceful 종료 (시스템 cron 불필요)
- KIS 키 설정 시 서버 시작과 함께 자동 실행

**[Feature] 프론트엔드 UI 개선**
- Orders: 각 행마다 삭제 버튼 + "KIS 동기화" 버튼 추가 (수동 즉시 동기화)
- KISLogs: 각 카드마다 삭제 버튼 추가

**Files Touched:**
- `backend/internal/agent/history.go` — `StartOrderSyncScheduler()` 추가
- `backend/cmd/server/main.go` — KIS 키 있을 때 스케줄러 자동 시작
- `backend/internal/api/handlers.go` — `GetOrders` sync 파라미터, `DeleteOrder`, `DeleteKISLog` 핸들러 추가
- `backend/internal/api/router.go` — DELETE 라우트 2개 추가
- `frontend/src/pages/Orders.jsx` — 삭제 버튼, KIS 동기화 버튼
- `frontend/src/pages/KISLogs.jsx` — 삭제 버튼

**Pending/Next Steps:**
- 삭제 시 확인 다이얼로그(confirm) 추가 여부 검토

---

## 2026-02-25 (2)

### [Fix] KIS API 버그 수정 및 계좌잔액/주문내역/포지션 전면 개선

**Description:**
보고된 Critical/Important/Minor 버그들을 모두 수정했습니다.

**[Critical] APBK0013 오파싱 버그**
- `msg_cd != "MABC000"` 조건을 `rt_cd != "0"` 기준으로 교체
- `rt_cd="0"` 이 KIS 공식 성공 기준 (APBK0013, MABC000 등 계좌 유형별 msg_cd 무관)
- 성공 주문이 DB에 FAILED로 기록되는 버그 해결

**[Critical] KIS TR-ID 신규 코드 교체**
- 매수: `TTTC0802U` → `TTTC0012U`, 매도: `TTTC0801U` → `TTTC0011U`

**[Fix] 계좌잔액 전면 수정 (에이전트 + 프론트 동시)**
- 거래가능금액: `ord_psbl_cash`(D+2 정산) → `dnca_tot_amt`(예수금총금액)으로 수정
- 출금가능금액(`prvs_rcdl_excc_amt`, D+2) 신규 필드 추가
- `GetAvailableOrder()` 호출 제거 → `inquire-balance` 단일 호출로 통합
- 에이전트도 동일 `GetAccountBalance()` 사용이므로 동시 수정됨

**[Feature] 포지션 실시간 동기화**
- `GET /api/positions` 추가: `inquire-balance output1` 기반 실시간 보유 종목 조회
- 종목코드/종목명/보유수량/매입평균가/현재가/평가손익/평가수익률 반환

**[Feature] 주문내역 종목명 + 체결가 표시**
- `orders` 테이블에 `stock_name`, `filled_price` 컬럼 추가 (ALTER TABLE 마이그레이션)
- `GetOrderHistory()` 동기화 시 `prdt_name`→`stock_name`, `avg_prvs`→`filled_price` 저장
- `PARTIALLY_FILLED` 부분체결 상태 추가 (체결수량 < 주문수량 판별)

**[Fix] 5분봉 차트 범위 확대**
- 150분(2.5h) → 390분(장 전체 6.5h, 78개 5분봉)

**[Feature] 에러 로그 요약 모드**
- `GET /api/logs/kis?summary=true` 파라미터 추가 (`raw_response` 제외)

**[Fix] 프론트엔드 화면 수정**
- Dashboard: 거래가능금액/출금가능금액 카드 분리 표시
- Orders: 종목명+종목코드 표시, 체결가 강조(노란색), 상태 한글 레이블
- StatusBadge: PARTIALLY_FILLED(부분체결) 추가

**Files Touched:**
- `backend/internal/kis/client.go` — rt_cd 성공 판정, TR-ID 교체, HoldingItem/GetHoldings 추가, InquireBalanceOutput2에 WithdrawableAmt 추가
- `backend/internal/agent/balance.go` — TradableAmount/WithdrawableAmount 추가, GetAvailableOrder 제거
- `backend/internal/agent/history.go` — stock_name/filled_price 동기화, PARTIALLY_FILLED 판별
- `backend/internal/agent/chart.go` — 5m 범위 150→390분
- `backend/internal/models/models.go` — StockName, FilledPrice, PARTIALLY_FILLED 추가
- `backend/internal/database/db.go` — orders 컬럼 추가, ALTER TABLE 마이그레이션
- `backend/internal/api/handlers.go` — GetPositions 핸들러, 로그 summary 파라미터
- `backend/internal/api/router.go` — /api/positions 라우트 추가
- `frontend/src/pages/Dashboard.jsx` — 거래가능/출금가능금액 카드 분리
- `frontend/src/pages/Orders.jsx` — 종목명, 체결가 표시
- `frontend/src/components/StatusBadge.jsx` — PARTIALLY_FILLED, 한글 레이블

**Pending/Next Steps:**
- 실제 KIS API 연동 후 `dnca_tot_amt` vs KIS 앱 "거래가능금액" 수치 재확인
- 포지션 페이지(`/positions`) 프론트엔드 UI 추가 (현재 API만 구현됨)

---

## 2026-02-25

### [Feature] 종목 현재가 + MA5/MA20 + 캔들 차트 API 추가

**Description:**
에이전트가 HTTP로 종목 정보와 차트 데이터를 조회할 수 있는 두 엔드포인트를 추가했습니다.
- `GET /api/stock/:code` — 현재가, 등락률, 거래량, MA5, MA20 반환
- `GET /api/stock/:code/chart?interval=1m|5m|1h` — 당일 장중 OHLCV 캔들 반환 (5m/1h는 1분봉에서 집계)

**Files Touched:**
- `backend/internal/kis/chart.go` (신규) — KIS 분봉/일봉 차트 API 클라이언트 (`GetMinuteChart`, `GetDailyChart`)
- `backend/internal/agent/chart.go` (신규) — `GetChart` 함수 + 분봉 페이지네이션 + 5분/시간봉 집계 로직
- `backend/internal/agent/stock_info.go` — `StockInfo`에 `MA5`, `MA20` 필드 추가; `GetStockInfo`에서 일봉 조회 후 MA 계산
- `backend/internal/api/handlers.go` — `GetStock`, `GetStockChart` 핸들러 추가
- `backend/internal/api/router.go` — `GET /api/stock/:code`, `GET /api/stock/:code/chart` 라우트 추가

**MA 계산 방식:**
- 최근 40 calendar days의 일봉 종가(`stck_clpr`)를 KIS `inquire-daily-itemchartprice`로 조회
- 오름차순 정렬 후 MA5 = 마지막 5개 평균, MA20 = 마지막 20개 평균
- 데이터 부족 시 0 반환

**차트 데이터 방식:**
- KIS `inquire-time-itemchartprice` (TR: FHKST03010200)로 1분봉 취득 (페이지당 30봉)
- 5m: 최대 5회 호출 → 150 1분봉 → 30 5분봉으로 집계
- 1h: 최대 14회 호출 → 390 1분봉 → 당일 전체 시간봉으로 집계

---

### [Fix] 모의투자 제거 및 크레덴셜 변경 시 토큰 자동 무효화

**Description:**
1. 모의투자(모의계좌 없음) 관련 코드 전체 제거
2. KIS APP KEY/SECRET 변경 시 구 토큰이 재사용되는 버그 수정 — SHA-256 fingerprint 비교로 감지 후 자동 삭제

**Files Touched:**
- `backend/cmd/server/main.go` — `InvalidateIfCredentialsChanged()` 호출 추가, mock 파라미터 제거
- `backend/internal/config/config.go` — `KISIsMock`, `KISMockURL`, `BaseURL()` 제거
- `backend/internal/kis/token.go` — `isMock`/`SetMode()` 제거; `InvalidateIfCredentialsChanged()` 추가 (settings 테이블에 credentials hash 저장); `GetCurrentToken`/`saveToken` 단순화
- `backend/internal/kis/client.go` — `isMock`/`mockBaseURL`/`realBaseURL`/`SetMock()`/`IsMock()`/`trID()` 제거; 실전 TR ID 하드코딩 (`TTTC8908R`, `TTTC8434R`, `TTTC0802U`, `TTTC0801U`, `TTTC8001R`)
- `backend/internal/models/models.go` — `Token.IsMock` 필드 제거
- `backend/internal/database/db.go` — tokens 테이블에서 `is_mock` 컬럼 제거, ALTER TABLE 마이그레이션 제거
- `backend/internal/api/handlers.go` — `SetMode` 핸들러 제거; `GetSettings` 응답에서 `is_mock` 제거
- `backend/internal/api/router.go` — `PUT /api/settings/mode` 라우트 제거
- `frontend/src/pages/Settings.jsx` — 모의/실전 토글 UI 제거; 계좌 정보만 표시

### [Fix] KIS API 잔고 조회 및 TR ID 수정

**Description:**
- `inquire-balance` output2가 배열임을 반영해 파싱 수정 → 총평가금액 정상 표시
- `inquire-psbl-order`에 필수 파라미터 `ORD_DVSN=01` 추가
- 모의투자 TR ID(`VTTC*`) 사용으로 인한 "모의투자 TR이 아닙니다" 오류 수정

**Files Touched:**
- `backend/internal/kis/client.go` — `GetInquireBalance` output2 배열 파싱, `GetAvailableOrder` ORD_DVSN=01 파라미터 추가
- `backend/internal/agent/balance.go` — `GetInquireBalance` + `GetAvailableOrder` 두 엔드포인트 조합으로 잔고 계산

### [Fix] 서버 재시작 시 KIS 토큰 재사용 (EGW00133 방지)

**Description:**
- 서버 재시작마다 토큰을 새로 발급해 KIS의 1분당 1회 제한(EGW00133)에 걸리는 문제 해결
- `EnsureToken()` 도입: DB에 유효 토큰(잔여 1시간 이상)이 있으면 재사용, 없으면 새로 발급

**Files Touched:**
- `backend/internal/kis/token.go` — `EnsureToken()` 메서드 추가
- `backend/cmd/server/main.go` — 시작 시 `IssueToken` → `EnsureToken` 변경

### [Feature] CD 파이프라인 추가 (NCP 서버 자동 배포)

**Description:**
- GitHub Actions에서 main 브랜치 push 시 NCP 서버에 자동 배포
- linux/amd64 크로스 컴파일 → SCP 전송 → React dist rsync → systemctl restart

**Files Touched:**
- `.github/workflows/ci.yml` — CD 단계 추가 (stop/transfer/restart)
- `deploy/micro-trading.service` — systemd 서비스 유닛 파일

---

## 2026-02-24

### [Feature] 초기 프로젝트 전체 구축 (Phase 1–7)

**Description:** 자동화 AI 트레이딩 시스템의 전체 초기 구조를 구축했습니다. Go 백엔드, React 프론트엔드, SQLite DB, KIS API 통합, CI/CD 파이프라인을 포함합니다.

**Files Touched:**
- `.gitignore` — Go, Node, .env, SQLite 제외 설정
- `.env.example` — 환경변수 키 목록 (실제 값 없음)
- `.github/workflows/ci.yml` — Go 빌드/테스트 + React 린트/빌드 자동화
- `backend/go.mod` — Go 모듈 (`github.com/micro-trading-for-agent/backend`)
- `backend/cmd/server/main.go` — 서버 진입점 (graceful shutdown 포함)
- `backend/internal/config/config.go` — .env 기반 설정 관리
- `backend/internal/database/db.go` — SQLite 초기화 + 자동 마이그레이션
- `backend/internal/models/models.go` — DB 모델 구조체 정의
- `backend/internal/logger/logger.go` — 구조화 JSON 로깅 (KISError 필수 필드 포함)
- `backend/internal/kis/ratelimiter.go` — KIS TPS 제한 (15 req/s)
- `backend/internal/kis/token.go` — KIS OAuth 토큰 발급 + 20시간 자동 갱신
- `backend/internal/kis/client.go` — KIS API 클라이언트 (주가/잔고/주문/내역)
- `backend/internal/agent/stock_info.go` — 종목 정보 조회
- `backend/internal/agent/balance.go` — 계좌 잔고 조회 + DB 스냅샷
- `backend/internal/agent/order.go` — 주문 실행 + DB 저장
- `backend/internal/agent/history.go` — 주문 내역 조회 + 상태 동기화
- `backend/internal/api/handlers.go` — HTTP 핸들러 (6개 엔드포인트)
- `backend/internal/api/router.go` — gin 라우터 설정
- `frontend/` — Vite+React+TailwindCSS 전체 구조
- `docs/architecture.md` — 프로젝트 아키텍처 문서
- `docs/db_schema.md` — SQLite 스키마 문서

**Pending/Next Steps:**
- `backend/go.sum` 생성 필요: `cd backend && go mod download`
- `frontend/package-lock.json` 생성 필요: `cd frontend && npm install`
- `.env` 파일 생성 후 실제 KIS API 키 입력
- KIS 모의투자 환경에서 토큰 발급 테스트
- `go test ./...` 용 단위 테스트 파일 추가 (현재 없음)
