# MQTT 브로커 설치 및 알림 구독 가이드

> 대상: NCP Micro 서버에 Mosquitto 브로커 설치 후, 스마트폰이나 개인PC에서 실시간 거래 알림을 구독하는 구성

---

## 역할

MQTT는 **사용자 알림 전용**으로 사용됩니다.

```
NCP Server
  ├── Go 백엔드 → PUBLISH  (목표가 도달, 손절가 도달, 15:15 청산)
  └── 사용자 기기 → SUBSCRIBE  (알림 수신)
```

- 백엔드는 브로커와 **같은 서버(localhost)**에서 loopback으로 연결합니다.
- 자율 트레이딩 엔진이 모든 매매를 내부에서 자동 처리하므로 외부에서 주문 API를 호출할 필요가 없습니다.

---

## 1. Mosquitto 설치 (NCP Ubuntu)

```bash
sudo apt update
sudo apt install -y mosquitto mosquitto-clients

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
# Mosquitto 2.x에서 리스너별 인증 분리
per_listener_settings true

# 로컬 전용 — 익명 허용 (백엔드 → 브로커)
listener 1883 localhost
allow_anonymous true

# 외부 사용자용 포트 — 비밀번호 필수
listener 1884 0.0.0.0
allow_anonymous false
password_file /etc/mosquitto/passwd
```

> **주의**: `per_listener_settings true` 없이 `allow_anonymous false`를 설정하면 1883 포트도 인증을 요구해 백엔드 MQTT 연결이 실패합니다.

### 사용자 생성

```bash
sudo mosquitto_passwd -c /etc/mosquitto/passwd user1
# 추가 사용자는 -c 없이 (파일 덮어쓰기 방지)
# sudo mosquitto_passwd /etc/mosquitto/passwd user2
```

```bash
sudo systemctl restart mosquitto
```

### NCP 방화벽 인바운드 규칙 추가

NCP 콘솔 → ACG(접근제어그룹) → 인바운드 규칙 추가:

| 프로토콜 | 포트 | 허용 IP |
|----------|------|---------|
| TCP | 1884 | 구독할 기기 IP/32 |

---

## 3. 백엔드 환경변수 설정 (`.env`)

```env
# MQTT — 로컬 브로커에 인증 없이 연결 (서버 내부 loopback)
MQTT_BROKER_URL=tcp://localhost:1883
MQTT_CLIENT_ID=micro-trading-server
```

> `MQTT_BROKER_URL` 미설정 시 기본값 `tcp://localhost:1883`. 브로커 미연결이어도 서버 정상 기동, 알림은 로그로만 남습니다.

---

## 4. 연결 확인

### 브로커 구독 테스트 (서버에서)

```bash
mosquitto_sub -h localhost -p 1883 -t "trading/#" -v
```

### 백엔드 기동 후 MQTT 로그 확인

```bash
sudo journalctl -u trading-server -f | grep -i mqtt
```

정상 시:
```json
{"level":"INFO","msg":"MQTT connected","broker":"tcp://localhost:1883"}
```

---

## 5. 외부 기기에서 알림 구독

### mosquitto_clients 로 테스트

```bash
mosquitto_sub \
  -h NCP_SERVER_IP \
  -p 1884 \
  -u user1 \
  -P YOUR_PASSWORD \
  -t "trading/#" \
  -v
```

### Python 구독 예시

```python
import paho.mqtt.client as mqtt
import json

def on_connect(client, userdata, flags, rc):
    print(f"Connected: {rc}")
    client.subscribe("trading/#")

def on_message(client, userdata, msg):
    payload = json.loads(msg.payload.decode())
    event = payload.get("event")
    code  = payload.get("stock_code")
    name  = payload.get("stock_name")
    price = payload.get("trigger_price")
    pnl   = payload.get("profit_pct")

    if event == "TARGET_HIT":
        print(f"[목표가 달성] {name}({code}) @ {price:,.0f}원 | 수익률 {pnl:.1f}%")
    elif event == "STOP_HIT":
        print(f"[손절 실행] {name}({code}) @ {price:,.0f}원 | 수익률 {pnl:.1f}%")
    elif event == "LIQUIDATION":
        print(f"[15:15 청산] {name}({code}) @ {price:,.0f}원")

client = mqtt.Client()
client.username_pw_set("user1", "YOUR_PASSWORD")
client.on_connect = on_connect
client.on_message = on_message
client.connect("NCP_SERVER_IP", 1884, 60)
client.loop_forever()
```

---

## 6. MQTT 메시지 스펙

### 토픽

| 토픽 | 이벤트 | 발행 시점 |
|------|--------|----------|
| `trading/alert/{stock_code}` | `TARGET_HIT` | 현재가 ≥ 목표가 → 자동 매도 완료 |
| `trading/alert/{stock_code}` | `STOP_HIT` | 현재가 ≤ 손절가 → 자동 매도 완료 |
| `trading/liquidation` | `LIQUIDATION` | 15:15 전량 시장가 청산 |

### 페이로드 JSON 스펙

```json
{
  "event": "TARGET_HIT",
  "stock_code": "005930",
  "stock_name": "삼성전자",
  "trigger_price": 73100,
  "target_price": 73000,
  "stop_price": 69700,
  "sell_qty": 10,
  "profit_pct": 3.5,
  "profit_amount": 31000,
  "timestamp": "2026-03-06T10:30:00+09:00",
  "is_test": false
}
```

| 필드 | 타입 | 설명 |
|------|------|------|
| `event` | string | `TARGET_HIT` \| `STOP_HIT` \| `LIQUIDATION` |
| `stock_code` | string | 6자리 종목코드 |
| `stock_name` | string | 한글 종목명 |
| `trigger_price` | float | 알림을 촉발한 가격 (원); LIQUIDATION은 현재가 |
| `target_price` | float | 등록된 목표가 (원) |
| `stop_price` | float | 등록된 손절가 (원) |
| `sell_qty` | int | 실제 매도 수량 (0=매도 실패) |
| `profit_pct` | float | 수익률 (%). `(trigger - filled) / filled × 100` |
| `profit_amount` | float | 실현 손익 금액 (KRW); 음수=손실 |
| `timestamp` | string | KST RFC3339 |
| `is_test` | bool | `true`이면 디버그 테스트 메시지 (KIS 실매도 없음) |

---

## 7. 문제 해결

| 증상 | 원인 | 해결 |
|------|------|------|
| `MQTT connect to tcp://localhost:1883: ...` 로그 | 브로커 미기동 | `sudo systemctl start mosquitto` |
| 외부 기기 연결 실패 | 방화벽 차단 | NCP ACG 인바운드 1884 포트 확인 |
| 알림 미수신 | WebSocket 미연결 | `GET /api/server/status`에서 `ws_connected` 확인; 08:50~16:00 장 시간 확인 |
| `no AES key for tr_id=H0STCNI0` 로그 | 체결통보 구독 실패 | `KIS_HTS_ID` 환경변수 설정 확인 |
