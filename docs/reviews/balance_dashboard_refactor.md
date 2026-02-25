# 코드 리뷰: 대시보드 잔고 API 리팩토링

> 작성일: 2026-02-25
> 대상 파일: `client.go`, `balance.go`, `order.go`, `Dashboard.jsx`

---

## 개요

이번 변경은 크게 두 가지 목표를 가집니다.

1. **대시보드 단순화**: 잔고 화면을 위해 KIS API를 두 번 호출하던 구조를 TTTC8434R(주식잔고조회) 한 번으로 줄이고, 표시 항목도 KIS 앱과 동일한 기준(출금가능, 자산증감)으로 정렬했습니다.
2. **에이전트 주문 정확도 향상**: 종목을 선정한 뒤 TTTC8908R(매수가능조회)로 실제 주문 가능 수량을 확인한 후 주문하는 흐름을 만들었습니다.

---

## Go 백엔드 해설

### 1. `client.go` — 구조체(Struct) 변경

```go
// 변경 전
type AvailableOrderResponse struct {
    AvailableAmount string `json:"nrcvb_buy_amt"`
}

// 변경 후
type AvailableOrderResponse struct {
    OrderableQty  string `json:"ord_psbl_qty"`
    AvailableCash string `json:"ord_psbl_cash"`
}
```

**Go 개념 설명 — 구조체 태그(struct tag):**
`json:"ord_psbl_qty"` 부분을 **구조체 태그**라고 합니다. Go의 `encoding/json` 패키지가 JSON 데이터를 Go 구조체로 변환(언마샬링)할 때 어떤 JSON 키와 연결할지 알려주는 메타데이터입니다. KIS API가 `"ord_psbl_qty"` 라는 키로 응답을 보내면, Go는 이 태그를 읽고 `OrderableQty` 필드에 값을 채웁니다.

---

### 2. `client.go` — 함수 시그니처 변경

```go
// 변경 전: 파라미터 없음 (종목 미지정)
func (c *Client) GetAvailableOrder(ctx context.Context) (*AvailableOrderResponse, error)

// 변경 후: 종목코드 파라미터 추가
func (c *Client) GetAvailableOrder(ctx context.Context, stockCode string) (*AvailableOrderResponse, error)
```

**Go 개념 설명 — 메서드 리시버(Method Receiver):**
`(c *Client)` 부분은 이 함수가 `Client` 타입에 소속된 **메서드**임을 나타냅니다. `c`는 메서드 안에서 Client 인스턴스에 접근하는 변수입니다. `*Client`처럼 포인터로 받으면 Client의 내부 상태(appKey, accountNo 등)를 직접 읽을 수 있습니다.

---

### 3. `balance.go` — API 호출 횟수 감소

```go
// 변경 전: 두 번 호출
summary, _ := client.GetInquireBalance(ctx)   // TTTC8434R
avail, _   := client.GetAvailableOrder(ctx)    // TTTC8908R

// 변경 후: 한 번 호출
summary, err := client.GetInquireBalance(ctx)  // TTTC8434R만 사용
```

**설계 이유:**
TTTC8908R은 특정 종목에 대한 주문 가능 수량을 계산하는 API입니다. 종목코드 없이 호출하면 의미 있는 수량을 얻을 수 없습니다. 따라서 대시보드용 잔고 조회에서는 제외하고, 실제 주문 직전에만 호출하도록 역할을 분리했습니다.

---

### 4. `balance.go` — 수익률 직접 계산

```go
// KIS가 asst_icdc_erng_rt를 항상 "0.00000000"으로 반환하므로 직접 계산
assetChangeRate := "-"
if prevTotal > 0 {
    rate := assetChangeAmt / prevTotal * 100
    assetChangeRate = fmt.Sprintf("%.2f", rate)
}
```

**Go 개념 설명 — `fmt.Sprintf`:**
`Sprintf`는 문자열을 특정 형식으로 포맷팅합니다. `"%.2f"`는 소수점 2자리까지 표시하는 부동소수점 형식입니다. 결과를 직접 출력하지 않고 문자열로 반환합니다(`S`pringf = **S**tring + printf).

**왜 `"-"` 초기값?**
전일 데이터가 없거나(계좌 개설 첫날) 0인 경우 나눗셈이 불가능합니다. Go에서 0으로 나누기를 방지하기 위해 `prevTotal > 0` 조건을 먼저 확인하고, 불가능한 경우 `"-"` 문자열을 반환해 프론트엔드에서 대시(-)로 표시합니다.

---

### 5. `order.go` — `CheckOrderFeasibility` 함수 추가

```go
type OrderFeasibility struct {
    OrderableQty  int     // 주문 가능 수량 (0이면 주문 불가)
    AvailableCash float64 // 주문 가능 현금 (재선정 기준)
}

func CheckOrderFeasibility(ctx context.Context, client *kis.Client, stockCode string) (*OrderFeasibility, error) {
    resp, err := client.GetAvailableOrder(ctx, stockCode)
    // ...
    qty, _ := strconv.Atoi(resp.OrderableQty)
    cash, _ := strconv.ParseFloat(resp.AvailableCash, 64)
    return &OrderFeasibility{OrderableQty: qty, AvailableCash: cash}, nil
}
```

**Go 개념 설명 — `strconv` 패키지:**
KIS API는 숫자도 JSON 문자열(`"123"`)로 반환합니다. Go는 타입이 엄격하므로 문자열을 숫자로 명시적으로 변환해야 합니다.
- `strconv.Atoi("123")` → `123` (int 변환, "A to i" = ASCII to integer)
- `strconv.ParseFloat("123.45", 64)` → `123.45` (float64 변환, 64는 비트 정밀도)

두 번째 반환값 `_`는 **blank identifier**로, 에러를 무시한다는 의미입니다. 여기서는 변환 실패 시 기본값(0)이 반환되어 `OrderableQty == 0` → 주문 불가로 처리되므로 안전합니다.

**에이전트 사용 예시:**
```go
feasibility, err := CheckOrderFeasibility(ctx, client, "005930")
if feasibility.OrderableQty > 0 {
    PlaceOrder(ctx, client, db, PlaceOrderRequest{
        StockCode: "005930",
        Qty:       feasibility.OrderableQty,
        // ...
    })
} else {
    // feasibility.AvailableCash 기준으로 다른 종목 재선정
}
```

---

## React 프론트엔드 해설

### Dashboard.jsx — 카드 구성 변경

```jsx
// 변경 전 (5개 카드)
<Card title="총 평가금액"  value={fmt(data?.total_eval)} />
<Card title="거래가능금액" value={fmt(data?.tradable_amount)} />
<Card title="출금가능금액" value={fmt(data?.withdrawable_amount)} />
<Card title="매입 금액"   value={fmt(data?.purchase_amt)} />
<Card title="평가 손익"   value={fmt(data?.eval_profit_loss)} />

// 변경 후 (4개 카드)
<Card title="총 평가금액"    value={fmt(data?.total_eval)} />
<Card title="출금가능금액"   value={fmt(data?.withdrawable_amount)} />
<Card title="자산증감액"     value={fmt(changeAmt)} />
<Card title="자산증감수익률" value={`${changeRate}%`} />
```

**React 개념 설명 — 옵셔널 체이닝(`?.`):**
`data?.total_eval`은 `data`가 `null` 또는 `undefined`일 때 에러 없이 `undefined`를 반환합니다. API 로딩 중에는 `data`가 아직 없으므로, 이 문법 덕분에 로딩 전 렌더링에서 앱이 죽지 않습니다.

---

### 조건부 색상 처리

```jsx
const changeColor =
  changeRate && changeRate !== '-'
    ? parseFloat(changeRate) > 0
      ? 'text-red-400'   // 상승: 빨간색 (한국 주식 관습)
      : parseFloat(changeRate) < 0
      ? 'text-blue-400'  // 하락: 파란색
      : ''               // 보합: 기본색
    : ''
```

**React 개념 설명 — 삼항 연산자 중첩:**
`조건 ? 참값 : 거짓값` 형태의 **삼항 연산자**를 중첩해 사용했습니다. 한국 증시 관습(상승=빨강, 하락=파랑)에 맞게 Tailwind CSS 클래스를 동적으로 선택합니다. JSX에서는 `if`문을 직접 쓸 수 없어 이 패턴을 자주 사용합니다.

---

### 그리드 레이아웃 조정

```jsx
// 변경 전: 6컬럼
<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4">

// 변경 후: 4컬럼
<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
```

**Tailwind CSS 개념 설명 — 반응형 접두사:**
- `grid-cols-1`: 기본(모바일) — 1열
- `sm:grid-cols-2`: 화면 640px 이상 — 2열
- `lg:grid-cols-4`: 화면 1024px 이상 — 4열

카드 수가 5→4개로 줄었으므로 최대 컬럼도 6→4로 줄여 각 카드가 더 넓게 표시됩니다.

---

## 핵심 요약

| 개념 | 설명 |
|---|---|
| Go 구조체 태그 | `json:"필드명"` — JSON 키와 Go 필드 연결 |
| 메서드 리시버 | `(c *Client)` — 특정 타입에 소속된 함수 |
| `strconv.Atoi` / `ParseFloat` | KIS API 문자열 숫자 → Go 숫자 타입 변환 |
| blank identifier `_` | 에러/반환값 무시 (0 기본값으로 안전 처리) |
| React `?.` 옵셔널 체이닝 | 로딩 중 null 참조 에러 방지 |
| Tailwind 반응형 접두사 | `sm:`, `lg:` — 화면 크기별 스타일 분기 |

**설계 원칙:**
*"API 호출은 필요한 시점에, 필요한 데이터만"* — 대시보드는 TTTC8434R 1회, 주문 결정은 TTTC8908R 1회로 역할을 명확히 분리했습니다.
