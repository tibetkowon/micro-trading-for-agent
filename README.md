# micro-trading-for-agent

AI 에이전트가 KIS(한국투자증권) API를 통해 자동으로 주식 거래를 수행하는 시스템.
NCP Micro (1GB RAM) 환경에서 효율적으로 동작하도록 설계되었습니다.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, Gin, SQLite (go-sqlite3) |
| Realtime | KIS WebSocket (`gorilla/websocket`) |
| Alerting | MQTT (`paho.mqtt.golang`) + Mosquitto |
| Frontend | React 18, Vite, TailwindCSS |
| Database | SQLite (WAL mode) |
| CI/CD | GitHub Actions |

## Quick Start

### 1. 환경변수 설정

```bash
cp .env.example .env
# .env 파일에 KIS API 키와 계좌 정보를 입력하세요
```

`.env` 필수/선택 항목:

| 키 | 필수 | 설명 |
|----|------|------|
| `KIS_APP_KEY` | ✅ | KIS Open API 앱 키 |
| `KIS_APP_SECRET` | ✅ | KIS Open API 시크릿 |
| `KIS_ACCOUNT_NO` | ✅ | 계좌번호 (숫자만) |
| `KIS_ACCOUNT_TYPE` | | 계좌 종류 (`01`=종합, 기본값) |
| `KIS_BASE_URL` | | 실전: `https://openapi.koreainvestment.com:9443` |
| `KIS_HTS_ID` | | HTS 아이디 — 실시간 체결통보 수신 시 필요 |
| `MQTT_BROKER_URL` | | MQTT 브로커 주소 (기본: `tcp://localhost:1883`) |
| `MQTT_CLIENT_ID` | | MQTT 클라이언트 ID (기본: `micro-trading-server`) |

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

---

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
| GET | `/api/orders` | 주문 내역 조회 (`?sync=true` KIS 체결 동기화, `?days=N`) |
| POST | `/api/orders` | 주문 실행 (`target_pct`, `stop_pct` 포함 시 모니터 자동 등록) |
| POST | `/api/orders/:id/cancel` | KIS 미체결 주문 취소 |
| DELETE | `/api/orders/:id` | 주문 단건 삭제 |
| GET | `/api/orders/feasibility?code=` | 주문가능수량 / 주문가능금액 조회 |

주문 요청 예시 (목표가/손절가 포함):
```json
{
  "stock_code": "005930",
  "order_type": "BUY",
  "qty": 10,
  "price": 71000,
  "target_pct": 3.0,
  "stop_pct": 2.0
}
```

### 실시간 모니터링

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/monitor/positions` | 현재 모니터링 중인 포지션 목록 |
| DELETE | `/api/monitor/positions/:code` | 모니터링 포지션 수동 해제 |

### 서버 / 장 운영 상태

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/server/status` | 통합 서버 상태 (시장·WebSocket·모니터·예수금) |
| GET | `/api/market/status` | 장운영 여부 (KIS 영업일 기준, 당일 캐시) |

`GET /api/server/status` 응답 예시:
```json
{
  "market_open": true,
  "trading_enabled": true,
  "available_cash": 500000,
  "ws_connected": true,
  "monitored_count": 2
}
```

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

---

## 스케줄러 / 자동화 동작

| 시각/주기 | 동작 | 조건 |
|-----------|------|------|
| **08:50 KST** | KIS WebSocket 연결 + 체결통보 구독 | 평일 |
| **15:15 KST** | 모니터링 중인 포지션 전량 시장가 청산 | 평일 |
| **16:00 KST** | WebSocket 연결 해제 | 평일 |
| 5분 주기 | Order Sync (KIS 체결 내역 → DB 동기화) | 장 중에만 |
| **08:50 KST** | KIS 액세스 토큰 갱신 (WebSocket 연결과 동시) | 평일 |

---

## 실시간 알림 플로우 (MQTT)

```
에이전트 → POST /api/orders {target_pct: 3.0, stop_pct: 2.0}
         → KIS 매수 주문
         → monitor.Register(target=price×1.03, stop=price×0.98)

KIS WebSocket (H0STCNT0 체결가)
  현재가 ≥ 목표가 → KIS 시장가 매도 → MQTT TARGET_HIT {sell_qty, profit_amount}
  현재가 ≤ 손절가 → KIS 시장가 매도 → MQTT STOP_HIT  {sell_qty, profit_amount}
  → 에이전트 수신: sell_qty>0 확인 후 새 종목 탐색 재개

15:15 → 전량 시장가 청산 → MQTT LIQUIDATION {sell_qty, profit_amount, trigger_price=현재가}
  → 에이전트 수신: 일일 리포트 작성
```

MQTT 설치 및 에이전트 연동 방법: [`docs/guides/mqtt-setup.md`](docs/guides/mqtt-setup.md)

---

## Project Structure

자세한 구조와 각 패키지의 역할: [`docs/architecture.md`](docs/architecture.md)

DB 스키마 상세: [`docs/db_schema.md`](docs/db_schema.md)

---

## Security

- 모든 민감 정보 (API 키, 계좌번호)는 `.env` 파일로만 관리
- `.env` 파일은 `.gitignore`에 의해 절대 커밋되지 않습니다
- KIS API 에러는 `kis_api_logs` 테이블에 자동 기록됩니다
- MQTT 외부 포트 접근은 IP 화이트리스트 + 비밀번호 인증 권장

## License

Private
