---
name: ai-trading-agent description: micro-trading 백엔드 서버 API를 사용하여 주식 시장 분석, 잔고 확인 및 매매 주문을 수행합니다. 
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
- Order sync: 백엔드 스케줄러가 3분마다 KIS 체결 상태 자동 갱신

---

## Agent Decision Workflow

1. 순위 API로 종목 후보 선정 (거래량/체결강도/대량체결건수/이격도) — ETF·ETN·우선주·위험종목 자동 제외됨
2. `GET /api/positions`로 현재 보유 종목 확인 → 중복 매수 방지
3. `GET /api/balance`로 예수금/가용 예산 확인
4. 현재가·이동평균·5분봉 캔들로 기술적 분석
5. `GET /api/orders/feasibility?code={code}`로 주문가능수량 확인
6. `POST /api/orders`로 매수/매도 주문 제출
7. 체결 상태 모니터링 (`FILLED`, `PARTIALLY_FILLED`, `PENDING`, `FAILED`)

---

## Core API Endpoints

### 계좌 & 잔고

| Endpoint | 설명 |
|----------|------|
| `GET /api/balance` | 총평가금액, 출금가능금액, 자산증감액/수익률 |
| `GET /api/positions` | KIS 실시간 기준 현재 보유 종목 |

### 종목 정보

| Endpoint | 설명 |
|----------|------|
| `GET /api/stock/{code}` | 현재가, 등락률, 거래량, MA5, MA20 |
| `GET /api/stock/{code}/chart?interval={1m|5m|1h}` | 당일 OHLCV 캔들 (5m: 장 전체 78봉, 1h: 당일 전체 시간봉) |

### 주문

| Endpoint | 설명 |
|----------|------|
| `GET /api/orders/feasibility?code={code}` | 주문가능수량(orderable_qty), 주문가능현금(available_cash) |
| `POST /api/orders` | 매수/매도 주문 제출 |
| `GET /api/orders?limit=50&offset=0&sync=true` | 주문 내역 조회 (sync=true: KIS 체결 상태 즉시 반영) |
| `DELETE /api/orders/{id}` | 주문 레코드 삭제 |

**POST /api/orders 요청 바디:**
```json
{
  "stock_code": "005930",
  "stock_name": "삼성전자",
  "order_type": "BUY",
  "quantity": 1,
  "price": 0
}
```

### 순위 (Rankings)

> **공통 특성:** 최대 30건, 다음 페이지 없음
> **자동 제외:** ETF·ETN·우선주·투자위험/경고/주의·관리종목·정리매매·불성실공시·거래정지 종목 — 별도 파라미터 없이 항상 일반 보통주만 반환

| Endpoint | 주요 파라미터 | 설명 |
|----------|-------------|------|
| `GET /api/ranking/volume` | `market`(J/NX), `sort`(0=평균거래량/1=거래량증가율/2=거래회전율/3=거래대금순) | 거래량 순위 |
| `GET /api/ranking/strength` | `market`(0000=전체/0001=거래소/1001=코스닥/2001=코스피200) | 체결강도 상위 |
| `GET /api/ranking/exec-count` | `market`, `sort`(0=매수상위/1=매도상위) | 대량체결건수 상위 |
| `GET /api/ranking/disparity` | `market`, `period`(5/10/20/60/120), `sort`(0=상위/1=하위) | 이격도 순위 |

**공통 가격 필터 파라미터 (4개 API 모두 지원):**

| 파라미터 | 설명 |
|---------|------|
| `price_min` | 최솟값 직접 입력 (원), 빈값=전체 |
| `price_max` | 최댓값 직접 입력 (원), 빈값=전체 |
| `use_balance_filter=true` | 잔액 API로 예수금 자동 조회 → price_max로 설정 (예수금=0이면 미적용) |

**사용 예시:**
```
# 내 예수금으로 살 수 있는 종목만 거래량 순위 조회
GET /api/ranking/volume?use_balance_filter=true

# 5만원 이하 종목만 체결강도 순위 조회
GET /api/ranking/strength?price_max=50000

# 코스닥, 1만~10만원 종목 거래량 순위
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

## 주요 동작 원칙

- `GET /api/orders/feasibility` 응답에서 `orderable_qty == 0` → 해당 종목 포기, `available_cash` 기준으로 종목 재선정
- 순위 API 응답에서 종목 선정 후 `GET /api/stock/{code}`로 MA5/MA20 등 기술적 지표 추가 확인 권장
- 주문 후 `GET /api/orders?sync=true`로 체결 확인; `PARTIALLY_FILLED` 시 잔여 수량 추가 주문 여부 판단
