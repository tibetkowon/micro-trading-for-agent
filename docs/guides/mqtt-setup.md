# MQTT 브로커 설치 및 에이전트 연동 가이드

> 대상: NCP Micro 서버에 Mosquitto 브로커 설치 후, 개인PC(외부 에이전트)가 실시간 거래 알림을 구독하는 구성

---

## 아키텍처 개요

```
NCP Server (Mosquitto :1883)
  ├── Go 백엔드 → PUBLISH  (목표가/손절가 도달, 청산)
  └── 개인PC 에이전트 → SUBSCRIBE  (알림 수신 후 /api/orders 호출)
```

백엔드는 브로커와 **같은 서버(localhost)**에서 실행되며 인터넷을 통한 직접 publish가 아니라 loopback으로 연결한다. 외부 에이전트는 NCP 서버 IP로 inbound TCP 연결한다.

---

## 1. Mosquitto 설치 (NCP Ubuntu)

```bash
sudo apt update
sudo apt install -y mosquitto mosquitto-clients

# 서비스 활성화 및 시작
sudo systemctl enable mosquitto
sudo systemctl start mosquitto
sudo systemctl status mosquitto
```

---

## 2. Mosquitto 설정

```bash
sudo nano /etc/mosquitto/conf.d/trading.conf
```

```conf
# Mosquitto 2.x에서 리스너별 인증을 분리하려면 반드시 필요
per_listener_settings true

# 로컬 전용 — 익명 허용 (백엔드 → 브로커)
listener 1883 localhost
allow_anonymous true

# 외부 에이전트용 포트 — 비밀번호 필수
listener 1884 0.0.0.0
allow_anonymous false
password_file /etc/mosquitto/passwd
```

> **주의**: `per_listener_settings true` 없이 `allow_anonymous false`를 설정하면 1883 포트도 인증을 요구해 백엔드 MQTT 연결이 실패한다.

### 사용자 생성

```bash
# agent / YOUR_PASSWORD 로 사용자 생성
sudo mosquitto_passwd -c /etc/mosquitto/passwd agent
# 이후 추가 사용자는 -c 없이 (파일 덮어쓰기 방지)
# sudo mosquitto_passwd /etc/mosquitto/passwd agent2
```

```bash
sudo systemctl restart mosquitto
```

### NCP 방화벽 인바운드 규칙 추가

NCP 콘솔 → ACG(접근제어그룹) → 인바운드 규칙 추가:

| 프로토콜 | 포트 | 허용 IP |
|----------|------|---------|
| TCP | 1884 | 개인PC IP/32 |

---

## 3. 백엔드 환경변수 설정 (`.env`)

```env
# MQTT — 로컬 브로커에 인증 없이 연결 (서버 내부 loopback)
MQTT_BROKER_URL=tcp://localhost:1883
MQTT_CLIENT_ID=micro-trading-server

# KIS WebSocket 체결통보용 HTS ID
KIS_HTS_ID=your_hts_id
```

> `MQTT_BROKER_URL` 미설정 시 기본값은 `tcp://localhost:1883`. 브로커 미연결이어도 서버는 정상 기동되며 알림은 로그로만 남는다.

---

## 4. 연결 확인

### 브로커 상태 확인

```bash
# 서버에서
mosquitto_sub -h localhost -p 1883 -t "trading/#" -v
```

### 백엔드 기동 후 MQTT 연결 로그 확인

```bash
sudo journalctl -u trading-server -f | grep -i mqtt
```

정상 시:
```json
{"level":"INFO","msg":"MQTT connected","broker":"tcp://localhost:1883"}
```

---

## 5. 외부 에이전트 (개인PC) 구독 설정

### mosquitto_clients 로 테스트

```bash
mosquitto_sub \
  -h NCP_SERVER_IP \
  -p 1884 \
  -u agent \
  -P YOUR_PASSWORD \
  -t "trading/#" \
  -v
```

### Python 에이전트 예시 (paho-mqtt)

```python
import paho.mqtt.client as mqtt
import json

BROKER = "NCP_SERVER_IP"
PORT   = 1884
USER   = "agent"
PASS   = "YOUR_PASSWORD"

def on_connect(client, userdata, flags, rc):
    print(f"Connected: {rc}")
    client.subscribe("trading/#")

def on_message(client, userdata, msg):
    payload = json.loads(msg.payload.decode())
    event = payload.get("event")
    code  = payload.get("stock_code")
    price = payload.get("trigger_price")

    if event == "TARGET_HIT":
        print(f"[목표가 도달] {code} @ {price:,.0f}원")
        # 필요 시 에이전트가 /api/orders 로 추가 매수/매도 요청

    elif event == "STOP_HIT":
        print(f"[손절가 도달] {code} @ {price:,.0f}원")
        # 백엔드가 포지션 제거 후 알림 발행; 에이전트는 기록만 해도 됨

    elif event == "LIQUIDATION":
        print(f"[15:15 청산] {code}")

client = mqtt.Client()
client.username_pw_set(USER, PASS)
client.on_connect = on_connect
client.on_message = on_message

client.connect(BROKER, PORT, 60)
client.loop_forever()
```

---

## 6. MQTT 메시지 스펙

### 토픽

| 토픽 | 이벤트 | 발행 시점 |
|------|--------|----------|
| `trading/alert/{stock_code}` | `TARGET_HIT` | 현재가 ≥ 목표가 |
| `trading/alert/{stock_code}` | `STOP_HIT` | 현재가 ≤ 손절가 |
| `trading/liquidation` | `LIQUIDATION` | 15:15 장마감 전량 청산 |
| `trading/report` | `DAILY_REPORT` | (예정) 일일 거래 리포트 |

### 페이로드 JSON 스펙

```json
{
  "event": "TARGET_HIT",
  "stock_code": "005930",
  "stock_name": "삼성전자",
  "trigger_price": 73100,
  "target_price": 73000,
  "stop_price": 70000,
  "profit_pct": 3.5,
  "timestamp": "2026-03-03T10:30:00+09:00"
}
```

| 필드 | 타입 | 설명 |
|------|------|------|
| `event` | string | `TARGET_HIT` \| `STOP_HIT` \| `LIQUIDATION` \| `DAILY_REPORT` |
| `stock_code` | string | 6자리 종목코드 |
| `stock_name` | string | 한글 종목명 |
| `trigger_price` | float | 알림을 촉발한 실제 가격 (원) |
| `target_price` | float | 등록된 목표가 (원) |
| `stop_price` | float | 등록된 손절가 (원) |
| `profit_pct` | float | 수익률 (%). `(trigger_price - filled_price) / filled_price × 100` |
| `timestamp` | string | KST RFC3339 형식 |

---

## 7. 모니터링 등록 방법 (에이전트 주문 플로우)

에이전트가 주문 시 `target_pct`와 `stop_pct`를 함께 전송하면 백엔드가 자동으로 모니터링을 등록한다.

```bash
curl -X POST http://NCP_SERVER_IP:8080/api/orders \
  -H "Content-Type: application/json" \
  -d '{
    "stock_code": "005930",
    "order_type": "BUY",
    "qty": 10,
    "price": 71000,
    "target_pct": 3.0,
    "stop_pct": 2.0
  }'
```

응답:
```json
{
  "OrderID": 42,
  "KISOrderID": "0000123456",
  "Status": "PENDING"
}
```

이후 `GET /api/monitor/positions` 에서 등록 확인 가능.

---

## 8. 모니터링 포지션 수동 관리

```bash
# 현재 모니터링 중인 포지션 목록
curl http://NCP_SERVER_IP:8080/api/monitor/positions

# 특정 종목 모니터링 해제
curl -X DELETE http://NCP_SERVER_IP:8080/api/monitor/positions/005930
```

---

## 9. 서버 통합 상태 확인

```bash
curl http://NCP_SERVER_IP:8080/api/server/status
```

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
| `ws_connected` | KIS WebSocket 연결 상태 |
| `monitored_count` | 현재 모니터링 중인 포지션 수 |

---

## 10. 문제 해결

| 증상 | 원인 | 해결 |
|------|------|------|
| `MQTT connect to tcp://localhost:1883: ...` 로그 | 브로커 미기동 | `sudo systemctl start mosquitto` |
| 외부 에이전트 연결 실패 | 방화벽 차단 | NCP ACG 인바운드 1884 포트 확인 |
| 알림 미수신 | WebSocket 미연결 | 장 시간(08:50~16:00) 확인; `ws_connected` API 체크 |
| `no AES key for tr_id=H0STCNI0` | 체결통보 구독 실패 | `KIS_HTS_ID` 환경변수 설정 확인 |
| 목표/손절가 0원 | `target_pct`/`stop_pct` 미전송 | 주문 시 파라미터 포함 여부 확인 |
