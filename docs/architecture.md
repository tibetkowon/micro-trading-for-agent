# Project Architecture

> Last updated: 2026-02-24

## Directory Tree

```
micro-trading-for-agent/
├── .github/
│   └── workflows/
│       └── ci.yml              # CI/CD pipeline (Go build/test + React lint/build)
├── .claude/
│   └── skills/                 # Behavioral skill instructions for the AI agent
├── backend/                    # Go backend root
│   ├── cmd/
│   │   └── server/
│   │       └── main.go         # Application entry point; wires all dependencies
│   ├── internal/
│   │   ├── config/
│   │   │   └── config.go       # .env loading via godotenv; exposes Config struct
│   │   ├── database/
│   │   │   └── db.go           # SQLite initialization + schema migration
│   │   ├── models/
│   │   │   └── models.go       # Shared Go structs matching DB tables
│   │   ├── logger/
│   │   │   └── logger.go       # Structured JSON logging; KISError() enforces required fields
│   │   ├── kis/
│   │   │   ├── client.go       # KIS REST API client (price, balance, order, history)
│   │   │   ├── token.go        # OAuth token issuance + 20-hour auto-refresh
│   │   │   └── ratelimiter.go  # TPS limiter (15 req/s) using golang.org/x/time/rate
│   │   ├── agent/
│   │   │   ├── stock_info.go   # Fetch stock price data for AI decision-making
│   │   │   ├── balance.go      # Account balance fetch + DB snapshot
│   │   │   ├── order.go        # Place buy/sell orders; persist to orders table
│   │   │   └── history.go      # Sync KIS execution history to local DB
│   │   └── api/
│   │       ├── handlers.go     # HTTP handler functions (one per endpoint)
│   │       └── router.go       # gin.Engine setup; route registration
│   ├── data/                   # SQLite .db files (git-ignored)
│   └── go.mod                  # Go module definition
├── frontend/                   # React frontend root
│   ├── src/
│   │   ├── main.jsx            # React entry point; BrowserRouter setup
│   │   ├── App.jsx             # Root component; navigation + route definitions
│   │   ├── index.css           # Tailwind base styles
│   │   ├── hooks/
│   │   │   └── useApi.js       # Generic fetch hook (loading/error/data/refetch)
│   │   ├── components/
│   │   │   ├── Card.jsx        # Reusable stat card
│   │   │   └── StatusBadge.jsx # Order status badge with color coding
│   │   └── pages/
│   │       ├── Dashboard.jsx   # Balance / profit rate cards
│   │       ├── Orders.jsx      # Order history table
│   │       ├── KISLogs.jsx     # KIS API error log viewer
│   │       └── Settings.jsx    # KIS credentials management UI
│   ├── index.html              # Vite HTML template
│   ├── vite.config.js          # Vite config; /api proxy to :8080
│   ├── tailwind.config.js      # Tailwind content paths
│   ├── postcss.config.js       # PostCSS (Tailwind + autoprefixer)
│   └── package.json            # npm dependencies
├── docs/
│   ├── architecture.md         # This file
│   ├── db_schema.md            # SQLite table documentation
│   ├── changelog.md            # Chronological change history
│   ├── plans/                  # Feature planning documents
│   └── reviews/                # Korean code review documents
├── .env.example                # Environment variable template (no secrets)
├── .gitignore                  # Excludes .env, *.db, node_modules, binaries
├── CLAUDE.md                   # Project instructions for the AI agent
└── README.md                   # Project overview
```

## Component Responsibilities

### `backend/internal/config`
- **Role:** Loads all configuration from environment variables (never hardcoded).
- **Put here:** Env key parsing, default values, derived helpers (e.g., `BaseURL()`).
- **Do NOT put here:** Business logic, DB queries, HTTP handlers.

### `backend/internal/database`
- **Role:** Opens the SQLite connection and runs schema migrations automatically on startup.
- **Put here:** `sql.DB` wrapper, migration SQL, connection pool settings.
- **Do NOT put here:** Business logic, raw queries used by business code.

### `backend/internal/models`
- **Role:** Shared data structures that map 1-to-1 with DB tables.
- **Put here:** Plain Go structs, constants for enum-like types.
- **Do NOT put here:** Methods with business logic, DB interactions.

### `backend/internal/logger`
- **Role:** Structured JSON logging. `KISError()` enforces the mandatory fields (error_code, timestamp, raw_response) per CLAUDE.md.
- **Put here:** Log formatting, severity levels.
- **Do NOT put here:** Business logic, HTTP handling.

### `backend/internal/kis`
- **Role:** KIS API integration. Handles authentication, rate limiting, and raw HTTP calls.
- **Put here:** Token management, rate limiter, API request/response DTOs, error logging to `kis_api_logs`.
- **Do NOT put here:** Business/trading logic, DB schema, API routing.

### `backend/internal/agent`
- **Role:** AI agent action functions. Bridges KIS API data with DB persistence for the trading loop.
- **Put here:** `GetStockInfo`, `GetAccountBalance`, `PlaceOrder`, `GetOrderHistory`.
- **Do NOT put here:** HTTP routing, raw KIS API calls (use `kis.Client`).

### `backend/internal/api`
- **Role:** HTTP layer. Thin handlers that validate input, call agent/DB functions, and return JSON.
- **Put here:** Route registration, request binding, response formatting, middleware.
- **Do NOT put here:** Business logic, direct DB queries beyond simple reads.

### `frontend/src/pages`
- **Role:** Top-level React views, one per route.
- **Put here:** Page layout, data fetching via `useApi`, user interaction logic.
- **Do NOT put here:** Reusable UI primitives (use `components/`).

### `frontend/src/components`
- **Role:** Reusable, stateless UI building blocks.
- **Put here:** `Card`, `StatusBadge`, and future shared widgets.
- **Do NOT put here:** Page-specific logic, API calls.
