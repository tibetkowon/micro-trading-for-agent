---
name: ai-trading-agent 
description: micro-trading 백엔드 서버 API를 사용하여 주식 시장 분석, 잔고 확인 및 매매 주문을 수행합니다.
when: 사용자가 "주식 사줘", "잔고 어때?", "종목 추천해줘" 등 트레이딩 관련 명령을 내릴 때
---

# AI 트레이딩 에이전트 운영 가이드

이 문서는 micro-trading 백엔드 서버와 통신하는 AI 트레이딩 에이전트를 위한 운영 가이드입니다.

---

## 서버 정보 및 거래 환경 제약

**Server Information:**
- Base URL: `http://223.130.143.83:8080`
- Authentication: None required (KIS API handled by backend)

**Trading Environment Constraints:**
- 거래 시간: 평일 09:00–15:30 KST (시간 외 주문 실패)
- API Rate Limit: 15 TPS (백엔드 자동 관리)
- Token Management: 20시간 자동 갱신
- Order types: `price=0` (시장가), `price>0` (지정가)
- Order sync: 백엔드 스케줄러가 3분마다 KIS 체결 상태 자동 갱신 + **수동 거래 자동 임포트** (사용자가 KIS 앱에서 직접 거래한 내역 포함). **단, 주말·공휴일·장 외 시간에는 자동 skip.**
- 장운영일 체크: 에이전트 행동 전 `GET /api/market/status`로 장 상태를 먼저 확인할 것. `is_open: false`이면 주문/조회 등 모든 거래 행동 금지.

---

## Agent Decision Workflow

### 매수 진입 흐름

1. **장운영 확인 (필수 첫 단계)** — `GET /api/market/status`
   - `is_open: true` → 진행
   - `is_open: false` → **즉시 중단** (reason: `weekend`/`outside_hours`/`holiday`/`check_failed`)
2. **종목 후보 선정** — 순위 API(`/ranking/volume`, `/ranking/strength`, `/ranking/exec-count`, `/ranking/disparity`)로 유망 종목 리스트 확보. ETF·ETN·우선주·위험종목은 자동 제외됨.
3. **중복 매수 방지** — `GET /api/positions`로 현재 보유 종목 확인. 이미 보유 중이면 패스.
4. **예산 확인** — `GET /api/balance`로 출금가능금액(예수금) 조회. 예산 부족 시 종목 재선정.
5. **기술적 분석** — `GET /api/stock/{code}`로 아래 지표 확인:
   - `trading_value` (거래대금): 높을수록 실질 자금 유입 큰 종목 → **우선 순위 부여**
   - `ma5` vs `ma20`: ma5 > ma20이면 단기 상승 추세
   - `rsi14`: **< 30** 과매도(반등 기대), **> 70** 과매수(진입 자제)
   - `macd_histogram`: **> 0 & 상승 전환** = 강세 신호, **< 0 & 하락** = 약세 신호
   - 값이 `0`이면 데이터 부족 → 해당 지표 판단 보류 (장 시작 직후 등)
6. **주문가능수량 확인** — `GET /api/orders/feasibility?code={code}`
   - `orderable_qty > 0`: 주문 가능
   - `orderable_qty == 0`: 해당 종목 포기, `available_cash` 기준으로 종목 재선정
7. **주문 제출** — `POST /api/orders` (매수)
8. **체결 확인** — `GET /api/orders?sync=true`
   - `FILLED`: 완전 체결 → 완료
   - `PARTIALLY_FILLED`: 잔여 수량 추가 주문 여부 판단
   - `PENDING`: 미체결 지속 시 취소 검토

### 미체결 주문 취소 흐름

- `POST /api/orders/{id}/cancel` 호출 → 백엔드가 KIS 취소가능조회(TTTC0084R) 후 취소(TTTC0013U) 자동 처리
- **주의:** `DELETE /api/orders/{id}`는 로컬 DB 레코드만 지우며, KIS 실제 취소가 아님

### 매도 흐름

1. `GET /api/positions`로 보유 종목 확인
2. `GET /api/stock/{code}`로 기술적 지표 재확인 (RSI > 70이거나 MACD 하락 전환 시 매도 고려)
3. `POST /api/orders` (`order_type: "SELL"`)로 매도 주문

---

## Core API Endpoints

### 장운영 상태 (모든 거래 행동 전 필수 확인)

| Endpoint | 설명 |
|----------|------|
| `GET /api/market/status` | 장운영 여부 조회. `is_open: false`이면 거래 행동 금지. |

**응답 예시:**
```json
{ "is_open": false, "checked_at": "2026-03-02T09:30:00+09:00", "reason": "holiday" }
```

| reason | 의미 | 에이전트 행동 |
|--------|------|-------------|
| `open` | 장 운영 중 | 거래 진행 가능 |
| `weekend` | 주말 | 모든 거래 중단 |
| `outside_hours` | 장 외 시간 (09:00 전 또는 15:30 후) | 모든 거래 중단 |
| `holiday` | 공휴일/임시 휴장 | 모든 거래 중단 |
| `check_failed` | KIS 영업일 조회 실패 | 모든 거래 중단 (fail-safe) |

---

### 계좌 & 잔고

| Endpoint | 설명 |
|----------|------|
| `GET /api/balance` | 총평가금액, 출금가능금액, 자산증감액/수익률 |
| `GET /api/positions` | KIS 실시간 기준 현재 보유 종목 |

### 종목 정보

| Endpoint | 설명 |
|----------|------|
| `GET /api/stock/{code}` | 현재가, 등락률, 거래량, 거래대금, MA5, MA20, RSI14, MACD(line/signal/histogram) |
| `GET /api/stock/{code}/chart?interval={1m\|5m\|1h}` | 당일 OHLCV 캔들 (5m: 장 전체 78봉, 1h: 당일 전체 시간봉) |

**GET /api/stock/{code} 응답 필드:**
```json
{
  "stock_code": "005930",
  "current_price": "75400",
  "change_rate": "1.21",
  "volume": "12345678",
  "trading_value": 929629327200,
  "ma5": 74820.0,
  "ma20": 73650.0,
  "rsi14": 58.32,
  "macd_line": 312.45,
  "macd_signal": 280.12,
  "macd_histogram": 32.33
}
```

| 필드 | 판단 기준 |
|------|-----------|
| `trading_value` | 클수록 자금 유입 큰 종목. 거래량보다 신뢰도 높음 |
| `rsi14` | < 30: 과매도(반등) / 30–70: 중립 / > 70: 과매수(진입 자제) |
| `macd_histogram` | > 0 & 상승: 매수 우호 / < 0 & 하락: 매수 자제 |
| `ma5 > ma20` | 단기 상승 추세 확인 |
| 모든 지표 = `0` | 데이터 부족 (장 시작 직후 등) — 판단 보류 후 재조회 |

### 주문

| Endpoint | 설명 |
|----------|------|
| `GET /api/orders/feasibility?code={code}` | 주문가능수량(orderable_qty), 주문가능현금(available_cash) |
| `POST /api/orders` | 매수/매도 주문 제출 |
| `POST /api/orders/{id}/cancel` | KIS 미체결 주문 취소 (TTTC0084R 확인 → TTTC0013U 취소) |
| `GET /api/orders?limit=50&offset=0&sync=true` | 주문 내역 조회 (sync=true: KIS 체결 상태 즉시 반영 + 수동 거래 임포트) |
| `DELETE /api/orders/{id}` | 주문 레코드 삭제 (로컬 DB에서만 제거, KIS 취소 아님) |

**POST /api/orders 요청 바디:**
```json
{
  "stock_code": "005930",
  "order_type": "BUY",
  "qty": 1,
  "price": 0
}
```

**POST /api/orders/{id}/cancel 응답:**
```json
{
  "order_id": 42,
  "kis_order_id": "0001569139",
  "status": "CANCELLED"
}
```
> 오류 시: `{"error": "..."}` — 이미 체결된 주문(FILLED), KIS 취소 불가, DB에 없는 주문 등

### 순위 (Rankings)

> **공통:** 최대 30건, ETF·ETN·우선주·위험종목 자동 제외

| Endpoint | 주요 파라미터 | 설명 |
|----------|-------------|------|
| `GET /api/ranking/volume` | `market`(J/NX), `sort`(0=평균거래량/1=증가율/2=회전율/3=거래대금순) | 거래량 순위 |
| `GET /api/ranking/strength` | `market`(0000=전체/0001=거래소/1001=코스닥/2001=코스피200) | 체결강도 상위 |
| `GET /api/ranking/exec-count` | `market`, `sort`(0=매수상위/1=매도상위) | 대량체결건수 상위 |
| `GET /api/ranking/disparity` | `market`, `period`(5/10/20/60/120), `sort`(0=상위/1=하위) | 이격도 순위 |

**공통 가격 필터 (4개 API 모두 지원):**

| 파라미터 | 설명 |
|---------|------|
| `price_min` | 최솟값 직접 입력 (원), 빈값=전체 |
| `price_max` | 최댓값 직접 입력 (원), 빈값=전체 |
| `use_balance_filter=true` | 예수금 자동 조회 → price_max로 설정 |

```
# 내 예수금으로 살 수 있는 종목만 거래량 순위 조회
GET /api/ranking/volume?use_balance_filter=true

# 코스닥, 1만~10만원 거래량 순위
GET /api/ranking/volume?market=NX&price_min=10000&price_max=100000
```

### 유틸리티

| Endpoint | 설명 |
|----------|------|
| `GET /api/logs/kis?summary=true` | KIS 에러 로그 (summary=true: raw_response 제외) |
| `DELETE /api/logs/kis/{id}` | 에러 로그 삭제 |
| `GET /api/settings` | 설정 조회 (read-only) |
| `GET /health` | 헬스 체크 |

---

## 주문 상태 레퍼런스

| 상태 | 의미 | 다음 액션 |
|------|------|----------|
| `PENDING` | KIS 접수 완료, 미체결 | 체결 대기 또는 취소 검토 |
| `FILLED` | 완전 체결 | 완료 |
| `PARTIALLY_FILLED` | 부분 체결 | 잔여 수량 추가 주문 여부 판단 |
| `CANCELLED` | 취소 완료 | 종목 재선정 가능 |
| `FAILED` | KIS 주문 실패 | 오류 원인 확인 후 재시도 |

## 주문 구분 (source 필드)

| source | 의미 | 에이전트 행동 |
|--------|------|-------------|
| `AGENT` | 에이전트가 직접 제출한 주문 | 정상 추적 |
| `MANUAL` | 사용자가 KIS 앱/웹에서 직접 거래한 내역 (자동 감지) | **매수/매도 의사결정 시 반드시 고려할 것** — 사용자가 이미 진입/청산한 포지션을 중복 주문하지 않도록 주의 |

> **중요:** `GET /api/orders?sync=true` 응답에서 `source=MANUAL` 주문이 포함될 수 있습니다. 에이전트는 이 내역을 포지션 판단에 반영해야 합니다.
