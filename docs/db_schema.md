# Database Schema

> Engine: SQLite (WAL mode, foreign keys enabled)
> Last updated: 2026-02-25

---

## Table: `settings`

**Purpose:** Stores application configuration key-value pairs. Currently used for internal system state (e.g., credential fingerprint). Sensitive values should be encrypted at the application layer in future iterations.

**Known keys:**
| Key | Description |
|-----|-------------|
| `kis_credentials_hash` | SHA-256 of `KIS_APP_KEY:KIS_APP_SECRET`; used on startup to detect credential changes and invalidate cached tokens |

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `key` | TEXT | PRIMARY KEY | Setting identifier (e.g., `KIS_APP_KEY`, `KIS_ACCOUNT_NO`) |
| `value` | TEXT | NOT NULL, DEFAULT '' | Setting value; sensitive keys are masked in API responses |
| `updated_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | Timestamp of last update |

---

## Table: `tokens`

**Purpose:** Persists KIS OAuth access tokens to survive server restarts. Only the most recent token (highest `id`) is used. All tokens are cleared automatically when `KIS_APP_KEY` or `KIS_APP_SECRET` changes.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | Surrogate key |
| `access_token` | TEXT | NOT NULL | KIS Bearer token string |
| `issued_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | When the token was issued |
| `expires_at` | DATETIME | NOT NULL | Token expiry (typically 24h after issue; refreshed at 20h) |

---

## Table: `orders`

**Purpose:** Full audit trail of every order placed by the AI agent or manual user request.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | Surrogate key |
| `stock_code` | TEXT | NOT NULL | KIS stock code (e.g., `005930` for Samsung Electronics) |
| `order_type` | TEXT | NOT NULL, CHECK IN ('BUY','SELL') | Direction of the trade |
| `qty` | INTEGER | NOT NULL, CHECK > 0 | Number of shares |
| `price` | REAL | NOT NULL, CHECK >= 0 | Order price per share; 0 = market order |
| `status` | TEXT | NOT NULL, DEFAULT 'PENDING', CHECK IN ('PENDING','FILLED','CANCELLED','FAILED') | Lifecycle status |
| `kis_order_id` | TEXT | NOT NULL, DEFAULT '' | KIS-assigned order number (`odno`); populated after submission |
| `created_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | Order creation timestamp |

---

## Table: `balances`

**Purpose:** Point-in-time snapshots of account balance. Used for historical trend analysis and as a fallback when KIS API is unavailable.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | Surrogate key |
| `total_eval` | REAL | NOT NULL, DEFAULT 0 | Total portfolio evaluation amount (KRW) |
| `available_amount` | REAL | NOT NULL, DEFAULT 0 | Cash available for new orders (KRW) |
| `recorded_at` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | Snapshot timestamp |

---

## Table: `kis_api_logs`

**Purpose:** Records every KIS API error response for debugging and compliance. Per CLAUDE.md, three fields are **mandatory**: `error_code`, `timestamp`, and `raw_response`.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | Surrogate key |
| `endpoint` | TEXT | NOT NULL | API endpoint path (e.g., `/uapi/domestic-stock/v1/trading/order-cash`) |
| `error_code` | TEXT | NOT NULL, DEFAULT '' | KIS `msg_cd` or HTTP status code string |
| `error_message` | TEXT | NOT NULL, DEFAULT '' | Human-readable error from KIS `msg1` field |
| `raw_response` | TEXT | NOT NULL, DEFAULT '' | Full raw JSON body from KIS API (required per CLAUDE.md) |
| `timestamp` | DATETIME | NOT NULL, DEFAULT `datetime('now')` | Exact time the error occurred (required per CLAUDE.md) |
