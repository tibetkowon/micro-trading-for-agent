# Plan: 수동 거래 내역 자동 임포트 + 정렬 개선

## Goal
KIS 동기화 시 로컬 DB에 없는 주문(사용자가 KIS 앱/웹에서 직접 체결한 수동 거래)을 자동으로 감지하여 `orders` 테이블에 삽입한다. 또한 정렬 기준을 실제 주문 시각 기준으로 수정하여 에이전트 주문과 수동 주문이 시간순으로 올바르게 표시되도록 한다.

## Requirements
- KIS 체결내역 조회(TTTC8001R) 결과 중, 로컬 DB에 없는 `odno`는 **새 레코드로 삽입**한다.
- 삽입 시 `source = 'MANUAL'`로 표시하여 에이전트 주문(`source = 'AGENT'`)과 구분한다.
- `created_at`을 KIS의 `ord_dt` + `ord_tmd` 기반 실제 주문 시각으로 설정하여 정렬이 올바르게 동작하게 한다.
- 정렬은 `created_at DESC` (동일 시각이면 `id DESC`)로 변경한다.
- 프론트엔드에서 `source`가 `MANUAL`인 레코드에 "수동" 배지를 표시한다.
- 중복 삽입 방지: 이미 존재하는 `kis_order_id`는 INSERT하지 않는다 (기존 UPDATE 로직 유지).

## Affected Files

| 파일 | 변경 내용 |
|------|-----------|
| `backend/internal/database/db.go` | `ALTER TABLE orders ADD COLUMN source` 마이그레이션 추가 |
| `backend/internal/models/models.go` | `OrderSource` 타입 + `AGENT`/`MANUAL` 상수, `Order.Source` 필드 추가 |
| `backend/internal/agent/history.go` | 수동 거래 INSERT 로직, `GetLocalOrderHistory` 정렬 수정, SELECT에 `source` 추가 |
| `frontend/src/pages/Orders.jsx` | "구분" 열 추가 — "에이전트" / "수동" 배지 표시 |

## Implementation Phases

### Phase 1: DB Schema — `source` 컬럼 추가
- `db.go`의 `alterStmts`에 아래 추가:
  ```sql
  ALTER TABLE orders ADD COLUMN source TEXT NOT NULL DEFAULT 'AGENT'
  ```
- 기존 레코드는 모두 `'AGENT'`로 자동 설정됨.

### Phase 2: Model — `Source` 필드
- `OrderSource` 타입과 `OrderSourceAgent = "AGENT"`, `OrderSourceManual = "MANUAL"` 상수 추가.
- `Order` 구조체에 `Source OrderSource \`json:"source"\`` 필드 추가.

### Phase 3: Backend 동기화 로직 개선
**`GetOrderHistory` 수정 (history.go):**
1. 각 KIS 주문에 대해 로컬 DB에 `kis_order_id`가 있는지 확인.
2. **없으면 INSERT** — 다음 KIS 필드를 파싱하여 사용:
   - `pdno` → `stock_code`
   - `prdt_name` → `stock_name`
   - `sll_buy_dvsn_cd` → `order_type` (01=SELL, 02=BUY)
   - `ord_unpr` → `price`
   - `avg_prvs` → `filled_price` (tot_ccld_qty > 0인 경우)
   - `ord_qty` → `qty`
   - `ord_dt` + `ord_tmd` → `created_at` (실제 주문 시각, "20060102"+"150405" 파싱)
   - `cncl_yn` + `tot_ccld_qty` + `ord_qty` → `status` 결정
   - `source = 'MANUAL'`
3. **있으면 기존 UPDATE 로직** 유지.

**`GetLocalOrderHistory` 수정:**
- SELECT에 `source` 컬럼 추가 및 `Scan` 수정.
- 정렬: `ORDER BY created_at DESC, id DESC`

### Phase 4: Frontend — 구분 배지
- 테이블 헤더에 "구분" 열 추가.
- `o.source === 'MANUAL'` → 주황색 "수동" 배지
- `o.source === 'AGENT'` 또는 없음 → 회색 "에이전트" 배지

## Verification
1. Go 빌드 성공: `go build ./...`
2. 프론트엔드 빌드 성공: `npm run build`
3. KIS 동기화 시 수동 거래가 `source=MANUAL`로 올바르게 삽입되는지 확인.
4. 정렬이 `created_at` 기준으로 올바른지 확인.
