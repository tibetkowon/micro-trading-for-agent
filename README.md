# micro-trading-for-agent

AI 에이전트가 KIS(한국투자증권) API를 통해 자동으로 주식 거래를 수행하는 시스템.
NCP Micro (1GB RAM) 환경에서 효율적으로 동작하도록 설계되었습니다.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.22, Gin, SQLite (go-sqlite3) |
| Frontend | React 18, Vite, TailwindCSS |
| Database | SQLite (WAL mode) |
| CI/CD | GitHub Actions |

## Quick Start

### 1. 환경변수 설정

```bash
cp .env.example .env
# .env 파일에 KIS API 키와 계좌 정보를 입력하세요
```

`.env` 필수 항목:

| 키 | 설명 |
|----|------|
| `KIS_APP_KEY` | KIS Open API 앱 키 |
| `KIS_APP_SECRET` | KIS Open API 시크릿 |
| `KIS_ACCOUNT_NO` | 계좌번호 (숫자만) |
| `KIS_ACCOUNT_TYPE` | 계좌 종류 (`01` = 종합, `22` = 선물옵션) |
| `KIS_BASE_URL` | 실전: `https://openapi.koreainvestment.com:9443` |
| `KIS_IS_MOCK` | `true` = 모의투자, `false` = 실전투자 |

### 2. 백엔드 실행

```bash
cd backend
go mod download
go run cmd/server/main.go
# → http://localhost:8080
```

### 3. 프론트엔드 실행

```bash
cd frontend
npm install
npm run dev
# → http://localhost:3000
```

## API Endpoints

### 계좌 / 잔고

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/balance` | 계좌 잔고 조회 |
| GET | `/api/positions` | 실시간 보유 종목 조회 |

### 종목 / 차트

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/stock/:code` | 종목 현재가 + MA5/MA20 |
| GET | `/api/stock/:code/chart` | 캔들 차트 (`?interval=1m\|5m\|1h`) |

### 주문

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orders` | 주문 내역 조회 (`?sync=true` KIS 체결 동기화, `?days=N` 범위) |
| POST | `/api/orders` | 수동 주문 (테스트용) |
| POST | `/api/orders/:id/cancel` | KIS 미체결 주문 취소 |
| DELETE | `/api/orders/:id` | 주문 단건 삭제 |
| GET | `/api/orders/feasibility?code=` | 주문가능수량 / 주문가능금액 조회 |

### 장운영 상태

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/market/status` | 장운영 여부 조회 (KIS 영업일 기준, 당일 캐시) |

응답 예시:
```json
{ "is_open": false, "checked_at": "2026-03-02T10:00:00+09:00", "reason": "holiday" }
```
`reason` 값: `open` / `weekend` / `outside_hours` / `holiday` / `check_failed`

### 순위

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/ranking/volume` | 거래량 순위 (`?sort=0~3`, `?price_min`, `?price_max`) |
| GET | `/api/ranking/strength` | 체결강도 순위 |
| GET | `/api/ranking/exec-count` | 대량체결건수 순위 |
| GET | `/api/ranking/disparity` | 이격도 순위 (`?period=5\|10\|20\|60\|120`) |

### 로그 / 설정

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/logs/kis` | KIS API 에러 로그 (`?summary=true` raw 제외) |
| DELETE | `/api/logs/kis/:id` | 에러 로그 단건 삭제 |
| GET | `/api/settings` | 설정 조회 (민감 정보 마스킹) |
| GET | `/health` | 헬스 체크 |

## 스케줄러 동작

| 스케줄러 | 주기 | 조건 |
|----------|------|------|
| Order Sync (`StartOrderSyncScheduler`) | 3분 | **장 중에만** 실행 (주말·공휴일·장 외 시간 자동 skip) |
| Token Auto Refresh | 20시간 | 항상 (KIS 토큰 만료 24시간 기준 선제 갱신) |

> **Order Sync 동작 원리**: KIS `inquire-daily-ccld` API (`TTTC0081R`)를 호출해 당일 체결 내역을 로컬 DB에 동기화합니다. 체결된 주문은 자동으로 `FILLED` / `PARTIALLY_FILLED` 상태로 업데이트되며, HTS/MTS에서 직접 체결된 수동 주문도 `MANUAL` 출처로 자동 인식합니다.

## Project Structure

자세한 구조와 각 패키지의 역할은 [`docs/architecture.md`](docs/architecture.md)를 참조하세요.

## Security

- 모든 민감 정보 (API 키, 계좌번호)는 `.env` 파일로만 관리
- `.env` 파일은 `.gitignore`에 의해 절대 커밋되지 않습니다
- KIS API 에러는 `kis_api_logs` 테이블에 자동 기록됩니다

## License

Private
