---
name: ai-trading-agent
description: |
  KIS(한국투자증권) Open API를 통한 자동 주식 거래 백엔드 서버.
  REST API로 종목 조회·주문 실행·잔고 확인을 수행하고,
  KIS WebSocket 실시간 가격 수신으로 목표가/손절가 도달 시 MQTT 알림을 발행한다.
  NCP Micro (1GB RAM) 환경에서 Go + SQLite로 경량 동작.
metadata:
  runtime: go
  entry: backend/cmd/server/main.go
  server_port: 8080
  required_env:
    - KIS_APP_KEY
    - KIS_APP_SECRET
    - KIS_ACCOUNT_NO
  optional_env:
    - KIS_ACCOUNT_TYPE     # 기본값: "01"
    - KIS_BASE_URL         # 기본값: https://openapi.koreainvestment.com:9443
    - KIS_HTS_ID           # 실시간 체결통보 수신용 (H0STCNI0)
    - MQTT_BROKER_URL      # 기본값: tcp://localhost:1883
    - MQTT_CLIENT_ID       # 기본값: micro-trading-server
    - DATABASE_PATH        # 기본값: ./data/trading.db
    - SERVER_PORT          # 기본값: 8080
---

# ai-trading-agent — 에이전트 스킬 레퍼런스

> AI 에이전트(외부 개인PC)가 이 서버를 도구로 사용하는 방법을 기술합니다.

---

## 서버 정보

| 항목 | 값 |
|------|-----|
| **Base URL** | `http://{NCP_SERVER_IP}:8080` |
| **거래 가능 시간** | 평일 09:00 ~ 15:30 KST |
| **WebSocket 운영** | 평일 08:50 연결 / 15:15 청산 / 16:00 해제 |
| **Order Sync** | 5분 주기 자동 동기화 (장 중에만) |
| **KIS 토큰 갱신** | 매일 08:50 WebSocket 연결과 함께 갱신 |

### 장운영 스케줄러

| 시각 (KST) | 동작 |
|------------|------|
| **08:50** | KIS WebSocket 연결 + 실시간 체결가(H0STCNT0) 구독 시작 |
| **15:15** | 모니터링 중인 포지션 전량 시장가 청산 → MQTT `trading/liquidation` 발행 |
| **16:00** | WebSocket 연결 해제 |

---

## Agent Decision Workflow

### 매수 진입 흐름

```
1. GET /api/server/status
   → market_open: true  (장 운영 중)
   → ws_connected: true (WebSocket 연결 상태 확인)
   → available_cash > 0 (예수금 확인)

2. GET /api/market/status
   → is_open: true 확인 (KIS 영업일 기준)

3. GET /api/orders/feasibility?code={code}
   → orderable_qty > 0
   → available_cash 충분 여부 확인

4. GET /api/ranking/volume (또는 strength / exec-count / disparity)
   → 거래량·체결강도·이격도 기준 종목 후보 선정

5. GET /api/stock/{code}
   → 현재가, MA5/MA20, RSI, MACD 분석
   → 매수 진입 조건 판단 (아래 지표 판단 기준 참조)

6. 매수 조건 충족 시:
   POST /api/orders
   { order_type: "BUY", stock_code, qty, price, target_pct, stop_pct }
   → target_pct + stop_pct 포함 시 체결 후 실시간 모니터링 자동 등록

7. MQTT subscribe (trading/alert/{code})
   → TARGET_HIT / STOP_HIT 알림 대기
   → 알림 수신 후 에이전트가 추가 판단 (SELL 주문 또는 기록)
```

### 미체결 주문 취소 흐름

```
1. GET /api/orders?sync=true&days=1
   → status: PENDING 주문 목록 확인

2. 오래된 PENDING 주문 발견 시:
   POST /api/orders/{id}/cancel
   → KIS 미체결 취소 요청
```

### 매도 흐름

```
[자동] 목표가/손절가 도달 시 — 서버가 직접 처리
   KIS WebSocket 가격 수신
   → HandlePrice() 목표/손절 조건 확인
   → KIS 시장가 매도 주문 (executeSell)
   → MQTT TARGET_HIT / STOP_HIT 발행
        { sell_qty: N, profit_amount: N, is_test: false }
   → 에이전트 수신 후 새 종목 탐색 시작

[자동] 15:15 서버 스케줄러
   → 전량 시장가 매도 (LiquidateAll)
   → MQTT LIQUIDATION 발행
        { sell_qty: N, profit_amount: N, trigger_price: 청산시점_현재가 }
   → 에이전트 수신 후 일일 리포트 작성

[수동] 에이전트 직접 매도
   POST /api/orders { order_type: "SELL", price: 0 } // 시장가
```

---

## Core API Endpoints

### 서버·장 상태

#### `GET /api/server/status` — 통합 서버 상태 ⭐ (매 거래 루프 시작 시 확인)

```json
{
  "market_open": true,
  "trading_enabled": true,
  "available_cash": 500000,
  "ws_connected": true,
  "monitored_count": 2
}
```

| 필드 | 설명 |
|------|------|
| `market_open` | 현재 장 운영 중 여부 |
| `trading_enabled` | 거래 가능 상태 (현재 market_open 동일) |
| `available_cash` | 주문가능현금 (KRW) |
| `ws_connected` | KIS WebSocket 연결 상태 — false면 실시간 모니터 비활성 |
| `monitored_count` | 현재 실시간 모니터링 중인 포지션 수 |

#### `GET /api/market/status` — KIS 영업일 기준 장운영 여부

```json
{
  "is_open": true,
  "checked_at": "2026-03-04T10:00:00+09:00",
  "reason": "open"
}
```

`reason`: `open` | `weekend` | `outside_hours` | `holiday` | `check_failed`

---

### 계좌 / 잔고

#### `GET /api/balance`

```json
{
  "total_eval": 1500000,
  "available_amount": 500000
}
```

#### `GET /api/positions` — 실시간 보유 종목

```json
[
  {
    "stock_code": "005930",
    "stock_name": "삼성전자",
    "qty": 10,
    "avg_price": 71000,
    "current_price": 73000,
    "profit_loss": 20000,
    "profit_loss_pct": 2.82
  }
]
```

#### `GET /api/orders/feasibility?code={code}` — 주문가능수량/금액

```json
{
  "orderable_qty": 14,
  "available_cash": 1023500
}
```

---

### 종목 정보

#### `GET /api/stock/{code}` — 현재가 + 보조지표

```json
{
  "stock_code": "005930",
  "stock_name": "삼성전자",
  "current_price": 73000,
  "open_price": 71500,
  "high_price": 73500,
  "low_price": 71200,
  "volume": 15234567,
  "trading_value": 1112580000000,
  "change_rate": 2.11,
  "ma5": 71800,
  "ma20": 70200,
  "rsi14": 62.5,
  "macd": 450.2,
  "macd_signal": 380.1
}
```

#### `GET /api/stock/{code}/chart?interval=1m|5m|1h` — 캔들 차트

---

### 주문

#### `POST /api/orders` — 주문 실행

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

| 필드 | 필수 | 설명 |
|------|------|------|
| `stock_code` | ✅ | 6자리 KIS 종목코드 |
| `order_type` | ✅ | `BUY` \| `SELL` |
| `qty` | ✅ | 주문 수량 (양의 정수) |
| `price` | ✅ | 주문 단가. `0` = 시장가 |
| `target_pct` | | 목표 수익률 %. BUY 시 포함하면 도달 시 MQTT `TARGET_HIT` 발행 |
| `stop_pct` | | 손절 비율 %. BUY 시 포함하면 도달 시 MQTT `STOP_HIT` 발행 |

> `target_pct > 0 && stop_pct > 0` 이면 체결 후 실시간 모니터링 자동 등록.
> 모니터링은 `GET /api/monitor/positions` 에서 확인.

응답:
```json
{
  "OrderID": 42,
  "KISOrderID": "0000123456",
  "Status": "PENDING"
}
```

#### `GET /api/orders?sync=true&days=1&limit=50` — 주문 내역 (KIS 동기화 포함)

#### `POST /api/orders/{id}/cancel` — KIS 미체결 주문 취소

#### `DELETE /api/orders/{id}` — 주문 단건 DB 삭제

---

### 실시간 모니터링

#### `GET /api/monitor/positions` — 모니터링 중인 포지션 목록

```json
[
  {
    "stock_code": "005930",
    "stock_name": "삼성전자",
    "filled_price": 71000,
    "target_price": 73130,
    "stop_price": 69580,
    "order_id": 42,
    "created_at": "2026-03-04T09:31:00+09:00"
  }
]
```

#### `DELETE /api/monitor/positions/{code}` — 모니터링 수동 해제

---

### 순위

```
GET /api/ranking/volume?use_balance_filter=true&sort=1
GET /api/ranking/strength?use_balance_filter=true
GET /api/ranking/exec-count?use_balance_filter=true
GET /api/ranking/disparity?period=20&sort=0
```

`use_balance_filter=true`: 예수금 기준 매수 가능한 가격대로 자동 필터

| 파라미터 | 설명 |
|----------|------|
| `sort` | 0=거래량순, 1=거래대금순, 2=등락율순, 3=종목코드순 |
| `period` | 이격도 기간: `5`\|`10`\|`20`\|`60`\|`120` |

---

### 유틸리티

```
GET  /api/logs/kis           # KIS API 에러 로그 (?summary=true raw 제외)
GET  /api/settings           # 설정 조회 (민감 정보 마스킹)
GET  /health                 # 헬스 체크
```

---

## 기술 지표 판단 기준

| 지표 | 매수 신호 | 매도 신호 | 참고 |
|------|-----------|-----------|------|
| **MA (이동평균)** | 현재가 > MA5 > MA20 (정배열) | 현재가 < MA5 < MA20 (역배열) | 추세 방향 판단 |
| **RSI14** | 30 이하 과매도 → 반등 기대 | 70 이상 과매수 → 조정 가능 | 0~100, 중립: 40~60 |
| **MACD** | MACD > Signal (골든크로스) | MACD < Signal (데드크로스) | 모멘텀 방향 |
| **이격도** | 20일선 이격도 낮음 (MA20 근접) | 이격도 과도하게 높음 | 평균회귀 활용 |
| **체결강도** | 100 이상 (매수 우세) | 100 미만 (매도 우세) | 순간 수급 판단 |

---

## 주문 상태 레퍼런스

| status | 의미 | 다음 행동 |
|--------|------|-----------|
| `PENDING` | 주문 접수, 미체결 | 모니터링 유지 또는 취소 |
| `FILLED` | 전량 체결 | target_pct/stop_pct 포함 시 모니터 자동 등록 |
| `PARTIALLY_FILLED` | 일부 체결 | 잔여 수량 모니터링 |
| `CANCELLED` | 취소 완료 | 재주문 검토 |
| `FAILED` | 주문 실패 | `GET /api/logs/kis` 에서 원인 확인 |

---

## `source` 필드 설명

| source | 의미 |
|--------|------|
| `AGENT` | AI 에이전트가 `POST /api/orders` 로 실행한 주문 |
| `MANUAL` | HTS/MTS에서 수동으로 실행한 주문 (Order Sync로 자동 감지) |

---

## MQTT 알림 수신

### 브로커 연결 (외부 에이전트)

```bash
mosquitto_sub \
  -h {NCP_SERVER_IP} \
  -p 1884 \
  -u agent \
  -P {PASSWORD} \
  -t "trading/#" \
  -v
```

### 토픽

| 토픽 | 이벤트 | 발행 시점 |
|------|--------|----------|
| `trading/alert/{stock_code}` | `TARGET_HIT` | 현재가 ≥ 목표가 |
| `trading/alert/{stock_code}` | `STOP_HIT` | 현재가 ≤ 손절가 |
| `trading/liquidation` | `LIQUIDATION` | 15:15 전량 청산 |

### 페이로드 JSON

```json
{
  "event": "TARGET_HIT",
  "stock_code": "005930",
  "stock_name": "삼성전자",
  "trigger_price": 73100,
  "target_price": 73130,
  "stop_price": 69580,
  "sell_qty": 10,
  "profit_pct": 2.96,
  "profit_amount": 21000,
  "timestamp": "2026-03-04T10:30:00+09:00",
  "is_test": false
}
```

| 필드 | 설명 |
|------|------|
| `trigger_price` | 목표/손절 도달 당시 가격 (LIQUIDATION: 청산 시점 현재가) |
| `sell_qty` | 실제 매도 수량. `0`이면 매도 미실행 (잔고 없음 또는 KIS 오류) |
| `profit_pct` | 수익률 % (음수=손실) |
| `profit_amount` | 실현 손익 금액 KRW (음수=손실) |
| `is_test` | `true`이면 장 외 디버그 메시지 — 실제 매도 없음 |

> 자세한 설정: `docs/guides/mqtt-setup.md`

---

## 에이전트 표준 거래 루프

```
1. GET /api/server/status
   → market_open && ws_connected 모두 true인지 확인
   → false면 대기 또는 종료

2. GET /api/market/status
   → is_open: true 확인

3. GET /api/orders/feasibility?code={후보종목}
   → orderable_qty > 0 && available_cash 충분한지 확인

4. GET /api/ranking/volume (또는 strength / disparity)
   → 종목 후보 선정

5. GET /api/stock/{code}
   → 현재가·MA5/MA20·RSI·MACD 분석 → 매수 조건 판단

6. POST /api/orders
   → { target_pct: 3.0, stop_pct: 2.0 } 포함 권장
   → 체결 후 서버가 자동으로 실시간 모니터 등록

7. MQTT subscribe (trading/alert/{code}, trading/liquidation)
   → TARGET_HIT / STOP_HIT 수신: 서버가 이미 매도 완료
        sell_qty > 0: 매도 성공 → 새 종목 탐색 루프로 복귀
        sell_qty = 0: 매도 실패 → GET /api/positions 확인 후 수동 처리
   → LIQUIDATION 수신: 장마감 청산 완료 → 일일 리포트 작성
```

---

## 스킬 목록 (개발자용 행동 지침)

| # | 스킬 파일 | 트리거 |
|---|-----------|--------|
| 1 | `.claude/skills/plan_feature.md` | 신규 기능 시작 전 |
| 2 | `.claude/skills/verify_implementation.md` | 코드 수정 후, 커밋 전 |
| 3 | `.claude/skills/record_changelog.md` | 구현 완료 후 |
| 4 | `.claude/skills/write_code_tutor.md` | 주요 코드 작성 후 |
| 5 | `.claude/skills/analyze_trade_logs.md` | KIS API 오류 조사 시 |
| 6 | `.claude/skills/update_db_schema.md` | SQLite 스키마 변경 시 |
| 7 | `.claude/skills/update_architecture.md` | 신규 패키지 추가 시 |
| 8 | `.claude/skills/update_readme.md` | 주요 마일스톤 후 |
| 9 | `.claude/skills/manage_skills.md` | 새 규칙·패턴 발견 시 |
| 10 | `.claude/skills/implement_kis_feature.md` | KIS API 기능 구현 시 |
| 11 | `.claude/skills/generate_openclaw_spec.md` | SKILL.md 갱신 시 |
