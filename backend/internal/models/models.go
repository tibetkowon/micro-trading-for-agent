package models

import "time"

// Setting stores key-value configuration pairs (e.g., KIS credentials).
type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrderType represents buy or sell.
type OrderType string

const (
	OrderTypeBuy  OrderType = "BUY"
	OrderTypeSell OrderType = "SELL"
)

// OrderStatus tracks the lifecycle of an order.
type OrderStatus string

const (
	OrderStatusPending         OrderStatus = "PENDING"
	OrderStatusFilled          OrderStatus = "FILLED"
	OrderStatusPartiallyFilled OrderStatus = "PARTIALLY_FILLED"
	OrderStatusCancelled       OrderStatus = "CANCELLED"
	OrderStatusFailed          OrderStatus = "FAILED"
)

// Order represents a single stock trade order.
type Order struct {
	ID          int64       `json:"id"`
	StockCode   string      `json:"stock_code"`
	StockName   string      `json:"stock_name"` // 종목명 (KIS 히스토리 동기화 시 채워짐)
	OrderType   OrderType   `json:"order_type"`
	Qty         int         `json:"qty"`
	Price       float64     `json:"price"`
	FilledPrice float64     `json:"filled_price"` // 체결가 (체결 후 avg_prvs 기준)
	Status      OrderStatus `json:"status"`
	KISOrderID  string      `json:"kis_order_id"`
	CreatedAt   time.Time   `json:"created_at"`
}

// Balance is a point-in-time snapshot of the account balance.
type Balance struct {
	ID              int64     `json:"id"`
	TotalEval       float64   `json:"total_eval"`
	AvailableAmount float64   `json:"available_amount"`
	RecordedAt      time.Time `json:"recorded_at"`
}

// KISAPILog records every KIS API error response for audit and debugging.
type KISAPILog struct {
	ID          int64     `json:"id"`
	Endpoint    string    `json:"endpoint"`
	ErrorCode   string    `json:"error_code"`
	ErrorMsg    string    `json:"error_message"`
	RawResponse string    `json:"raw_response"`
	Timestamp   time.Time `json:"timestamp"`
}

// Token stores the KIS OAuth access token and its validity window.
type Token struct {
	ID          int64     `json:"id"`
	AccessToken string    `json:"access_token"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}
