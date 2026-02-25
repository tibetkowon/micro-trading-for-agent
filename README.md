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

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/balance` | 계좌 잔고 조회 |
| GET | `/api/positions` | 실시간 보유 종목 조회 |
| GET | `/api/orders` | 주문 내역 조회 (`?sync=true` 로 KIS 체결 동기화) |
| POST | `/api/orders` | 수동 주문 (테스트용) |
| DELETE | `/api/orders/:id` | 주문 단건 삭제 |
| GET | `/api/logs/kis` | KIS API 에러 로그 (`?summary=true` 로 raw 제외) |
| DELETE | `/api/logs/kis/:id` | 에러 로그 단건 삭제 |
| GET | `/api/stock/:code` | 종목 현재가 + MA5/MA20 |
| GET | `/api/stock/:code/chart` | 캔들 차트 (`?interval=1m\|5m\|1h`) |
| GET | `/api/settings` | 설정 조회 (민감 정보 마스킹) |
| GET | `/health` | 헬스 체크 |

## Project Structure

자세한 구조와 각 패키지의 역할은 [`docs/architecture.md`](docs/architecture.md)를 참조하세요.

## Security

- 모든 민감 정보 (API 키, 계좌번호)는 `.env` 파일로만 관리
- `.env` 파일은 `.gitignore`에 의해 절대 커밋되지 않습니다
- KIS API 에러는 `kis_api_logs` 테이블에 자동 기록됩니다

## License

Private
