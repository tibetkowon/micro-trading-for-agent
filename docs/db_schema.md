# Database Schema

> Engine: SQLite (WAL mode, foreign keys enabled)
> Last updated: 2026-03-04 (rev 4 — Phase 1: monitored_positions, orders.target_pct/stop_pct)

---

## Table: `settings`

**Purpose:** 애플리케이션 설정 키-값 저장소. 현재는 자격증명 지문 캐시에 사용.

**Known keys:**
| Key | Description |
|-----|-------------|
| `kis_credentials_hash` | SHA-256 of `KIS_APP_KEY:KIS_APP_SECRET`; 서버 시작 시 자격증명 변경 감지 및 캐시 토큰 무효화에 사용 |

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `key` | TEXT | PRIMARY KEY | 설정 식별자 |
| `value` | TEXT | NOT NULL, DEFAULT '' | 설정값; 민감 키는 API 응답에서 마스킹 |
| `updated_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | 마지막 업데이트 시각 |

---

## Table: `tokens`

**Purpose:** KIS OAuth 액세스 토큰 영속화. 서버 재시작 시에도 토큰 유지. 가장 최근 토큰(highest `id`)만 사용. `KIS_APP_KEY`/`KIS_APP_SECRET` 변경 시 전체 무효화.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | 대리 키 |
| `access_token` | TEXT | NOT NULL | KIS Bearer 토큰 문자열 |
| `issued_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | 토큰 발급 시각 |
| `expires_at` | DATETIME | NOT NULL | 토큰 만료 시각 (보통 발급 후 24시간; 20시간 기준 선제 갱신) |

---

## Table: `orders`

**Purpose:** 모든 주문의 완전한 감사 로그. AI 에이전트 주문과 수동 감지 KIS 거래 모두 포함.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | 대리 키 |
| `stock_code` | TEXT | NOT NULL | KIS 종목코드 (e.g., `005930`) |
| `stock_name` | TEXT | NOT NULL, DEFAULT '' | 종목명 (e.g., `삼성전자`); `GetOrderHistory()` 동기화 시 채워짐 |
| `order_type` | TEXT | NOT NULL, CHECK IN ('BUY','SELL') | 매매 방향 |
| `qty` | INTEGER | NOT NULL, CHECK > 0 | 주문 수량 |
| `price` | REAL | NOT NULL, CHECK >= 0 | 주문 단가; 0 = 시장가 주문 |
| `filled_price` | REAL | NOT NULL, DEFAULT 0 | 평균 체결가 (`avg_prvs`); 체결 후 채워짐 |
| `status` | TEXT | NOT NULL, DEFAULT 'PENDING', CHECK IN ('PENDING','FILLED','PARTIALLY_FILLED','CANCELLED','FAILED') | 주문 생애주기 상태 |
| `kis_order_id` | TEXT | NOT NULL, DEFAULT '' | KIS 주문번호 (`odno`); 제출 후 채워짐 |
| `source` | TEXT | NOT NULL, DEFAULT 'AGENT' | 주문 출처: `AGENT`=AI 에이전트 / `MANUAL`=HTS/MTS 수동 거래 |
| `target_pct` | REAL | NOT NULL, DEFAULT 0 | 목표 수익률 (%). 0이면 모니터링 미등록 |
| `stop_pct` | REAL | NOT NULL, DEFAULT 0 | 손절 비율 (%). 0이면 모니터링 미등록 |
| `created_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | 주문 시각; MANUAL 주문은 KIS `ord_dt`+`ord_tmd` 기준으로 설정 |

**정렬 기준:** `created_at DESC, id DESC`

---

## Table: `balances`

**Purpose:** 계좌 잔고의 시점 스냅샷. 이력 추이 분석 및 KIS API 미가용 시 폴백으로 사용.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | 대리 키 |
| `total_eval` | REAL | NOT NULL, DEFAULT 0 | 총 평가금액 (KRW) |
| `available_amount` | REAL | NOT NULL, DEFAULT 0 | 거래가능금액 (`dnca_tot_amt` / 예수금총금액, KRW) |
| `recorded_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | 스냅샷 시각 |

---

## Table: `kis_api_logs`

**Purpose:** 모든 KIS API 오류 응답 기록. CLAUDE.md 기준으로 `error_code`, `timestamp`, `raw_response` 3개 필드는 **필수**.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | 대리 키 |
| `endpoint` | TEXT | NOT NULL | API 엔드포인트 경로 (e.g., `/uapi/domestic-stock/v1/trading/order-cash`) |
| `error_code` | TEXT | NOT NULL, DEFAULT '' | KIS `msg_cd` 또는 HTTP 상태 코드 문자열 |
| `error_message` | TEXT | NOT NULL, DEFAULT '' | KIS `msg1` 필드의 사람이 읽을 수 있는 오류 메시지 |
| `raw_response` | TEXT | NOT NULL, DEFAULT '' | KIS API 전체 원본 JSON 응답 (CLAUDE.md 필수) |
| `timestamp` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | 오류 발생 정확한 시각 (CLAUDE.md 필수) |

**보존 정책:** `GET /api/logs/kis` 호출 시 2일 이상 된 로그는 자동 삭제.

---

## Table: `monitored_positions`

**Purpose:** 실시간 모니터링 중인 매수 포지션. WebSocket 가격 이벤트로 목표가/손절가 도달 여부를 체크. 서버 재시작 시 이 테이블에서 포지션을 복구한다.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | 대리 키 |
| `stock_code` | TEXT | NOT NULL, UNIQUE | KIS 종목코드 (종목당 1개 포지션) |
| `stock_name` | TEXT | NOT NULL, DEFAULT '' | 종목명 |
| `filled_price` | REAL | NOT NULL, DEFAULT 0 | 체결가 (목표가/손절가 계산 기준) |
| `target_price` | REAL | NOT NULL, DEFAULT 0 | 목표가 (원). `filled_price × (1 + target_pct/100)` |
| `stop_price` | REAL | NOT NULL, DEFAULT 0 | 손절가 (원). `filled_price × (1 - stop_pct/100)` |
| `order_id` | INTEGER | NOT NULL, DEFAULT 0 | 연결된 `orders.id` (역추적용) |
| `created_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | 모니터링 등록 시각 |

**생애주기:**
1. `POST /api/orders` with `target_pct > 0 && stop_pct > 0` → INSERT
2. `HandlePrice()` 에서 목표가/손절가 도달 감지 → `monitor.Remove()` → DELETE
3. 15:15 `LiquidateAll()` → DELETE
4. `DELETE /api/monitor/positions/:code` → DELETE

---

## 마이그레이션 전략

새 컬럼 추가 시 `ALTER TABLE ... ADD COLUMN` 을 `alterStmts` 슬라이스에 추가한다. 이미 존재하는 컬럼에 대한 오류("duplicate column name")는 정상으로 무시된다. 새 테이블은 `CREATE TABLE IF NOT EXISTS` 로 `stmts` 슬라이스에 추가한다.
