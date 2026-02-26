# Plan: 종목 순위 API 구현 + 주문가능수량 엔드포인트 노출

**Date:** 2026-02-26
**Trigger:** 에이전트 종목 선정 지원 및 주문가능수량 조회 워크플로 구현

---

## Goal

에이전트가 종목 선정 단계에서 사용할 수 있도록 KIS 순위 API 4종을 HTTP 엔드포인트로 제공하고,
기존 내부 함수로만 존재하던 `CheckOrderFeasibility()`를 HTTP 엔드포인트로 노출한다.

### 에이전트 워크플로 (변경 후)

```
종목선정 (순위 API로 후보 확보)
  → GET /api/orders/feasibility?code=:code (주문가능수량 확인)
    → qty > 0: POST /api/orders (주문 제출)
    → qty = 0: available_cash 반환 → 예산 내 종목 재선정 → 반복
```

---

## 변경 범위

### 1. HTTP 엔드포인트 추가 — 주문가능수량

| Method | Path | 설명 |
|--------|------|------|
| `GET` | `/api/orders/feasibility?code=:code` | 종목코드로 주문가능수량·가능금액 조회 |

**응답 예시:**
```json
{
  "orderable_qty": 12,
  "available_cash": 1500000
}
```

- 기존 `CheckOrderFeasibility()` (agent/order.go:37)을 그대로 호출
- KIS TTTC8908R은 이미 구현됨 → 핸들러 + 라우트만 추가

---

### 2. KIS 순위 4종 신규 구현

각 API는 KIS → Agent → HTTP 3계층으로 구현.

#### A. 거래량 순위 `GET /api/ranking/volume`

- **TR_ID:** FHPST01710000
- **KIS URL:** `/uapi/domestic-stock/v1/quotations/volume-rank`
- **쿼리 파라미터:**
  - `market` (기본값: `J`) — J: KRX, NX: NXT
  - `sort` (기본값: `0`) — 0: 평균거래량, 1: 거래량증가율, 2: 평균거래회전율, 3: 거래대금순
- **응답 필드:** `data_rank`, `stock_code`, `stock_name`, `current_price`, `volume`, `avg_volume`, `vol_increase_rate`

#### B. 체결강도 순위 `GET /api/ranking/strength`

- **TR_ID:** FHPST01680000
- **KIS URL:** `/uapi/domestic-stock/v1/ranking/volume-power`
- **쿼리 파라미터:**
  - `market` (기본값: `0000`) — 0000: 전체, 0001: 거래소, 1001: 코스닥, 2001: 코스피200
- **응답 필드:** `data_rank`, `stock_code`, `stock_name`, `current_price`, `volume`, `strength` (체결강도), `buy_qty`, `sell_qty`

#### C. 대량체결건수 순위 `GET /api/ranking/exec-count`

- **TR_ID:** FHKST190900C0
- **KIS URL:** `/uapi/domestic-stock/v1/ranking/bulk-trans-num`
- **쿼리 파라미터:**
  - `market` (기본값: `0000`) — 0000: 전체, 0001: 거래소, 1001: 코스닥, 2001: 코스피200
  - `sort` (기본값: `0`) — 0: 매수상위, 1: 매도상위
- **응답 필드:** `data_rank`, `stock_code`, `stock_name`, `current_price`, `volume`, `buy_count`, `sell_count`, `net_buy_qty`

#### D. 이격도 순위 `GET /api/ranking/disparity`

- **TR_ID:** FHPST01780000
- **KIS URL:** `/uapi/domestic-stock/v1/ranking/disparity`
- **쿼리 파라미터:**
  - `market` (기본값: `0000`) — 0000: 전체, 0001: 거래소, 1001: 코스닥, 2001: 코스피200
  - `period` (기본값: `20`) — 5, 10, 20, 60, 120 (이격도 기준 일수)
  - `sort` (기본값: `0`) — 0: 이격도 상위, 1: 이격도 하위
- **응답 필드:** `data_rank`, `stock_code`, `stock_name`, `current_price`, `change_rate`, `volume`, `d5`, `d10`, `d20`, `d60`, `d120`

---

## 구현 파일 목록

| 파일 | 작업 |
|------|------|
| `backend/internal/kis/client.go` | `GetVolumeRank()`, `GetStrengthRank()`, `GetExecCountRank()`, `GetDisparityRank()` 함수 추가 |
| `backend/internal/kis/types.go` (또는 client.go 내 정의) | 순위 API 응답 구조체 4종 추가 |
| `backend/internal/agent/ranking.go` (신규) | Agent 레벨 wrapper 4종 + 공통 응답 구조체 |
| `backend/internal/api/handlers.go` | `HandleGetFeasibility`, `HandleGetVolumeRank`, `HandleGetStrengthRank`, `HandleGetExecCountRank`, `HandleGetDisparityRank` 핸들러 5종 추가 |
| `backend/internal/api/router.go` | 신규 라우트 5개 등록 |
| `SKILL.md` (프로젝트 루트) | 전체 엔드포인트 + 워크플로 업데이트 |

---

## 검증 계획

1. `go build ./...` — 컴파일 오류 없음 확인
2. `go test ./...` — 기존 테스트 통과 확인
3. 각 엔드포인트 라우트 등록 재검토
4. KIS API 파라미터가 명세와 일치하는지 재검토

---

## 커밋 전략

- 브랜치: `claude/update-skill-features-PPzET`
- 커밋 1: `feat: 주문가능수량 HTTP 엔드포인트 추가 (GET /api/orders/feasibility)`
- 커밋 2: `feat: KIS 순위 API 4종 HTTP 엔드포인트 추가 (거래량/체결강도/체결건수/이격도)`
- 커밋 3: `docs: SKILL.md 업데이트 - 신규 엔드포인트 및 에이전트 워크플로 반영 [skip actions]`
