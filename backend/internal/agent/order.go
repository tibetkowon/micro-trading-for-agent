package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

// PlaceOrderRequest contains the parameters for a new order.
type PlaceOrderRequest struct {
	StockCode string
	OrderType models.OrderType
	Qty       int
	Price     float64 // 0 for market order
	OrderDivn string  // "00"=지정가, "01"=시장가
}

// OrderFeasibility is returned by CheckOrderFeasibility.
type OrderFeasibility struct {
	OrderableQty  int     // TTTC8908R nrcvb_buy_qty (미수없는매수수량) — 0이면 주문 불가
	AvailableCash float64 // TTTC8908R ord_psbl_cash — 재선정 시 예산 기준
}

// CheckOrderFeasibility calls TTTC8908R for a specific stock and returns
// how many shares can be bought at market price.
//
// Agent flow:
//
//	qty, _ := CheckOrderFeasibility(ctx, client, "005930")
//	if qty.OrderableQty > 0 { PlaceOrder(..., qty.OrderableQty) }
//	else                     { re-select stock using qty.AvailableCash }
func CheckOrderFeasibility(ctx context.Context, client *kis.Client, stockCode string) (*OrderFeasibility, error) {
	resp, err := client.GetAvailableOrder(ctx, stockCode)
	if err != nil {
		return nil, fmt.Errorf("GetAvailableOrder(%s): %w", stockCode, err)
	}

	qty, _ := strconv.Atoi(resp.OrderableQty)
	cash, _ := strconv.ParseFloat(resp.AvailableCash, 64)

	return &OrderFeasibility{
		OrderableQty:  qty,
		AvailableCash: cash,
	}, nil
}

// PlaceOrderResult is returned after successfully submitting an order.
type PlaceOrderResult struct {
	OrderID    int64
	KISOrderID string
	Status     models.OrderStatus
}

// PlaceOrder submits a buy or sell order to KIS and records it in the DB.
func PlaceOrder(ctx context.Context, client *kis.Client, db *database.DB, req PlaceOrderRequest) (*PlaceOrderResult, error) {
	kisReq := kis.OrderRequest{
		StockCode: req.StockCode,
		OrderDivn: req.OrderDivn,
		Qty:       fmt.Sprintf("%d", req.Qty),
		Price:     fmt.Sprintf("%.0f", req.Price),
	}

	var (
		kisResp *kis.OrderResponse
		err     error
	)
	switch req.OrderType {
	case models.OrderTypeBuy:
		kisResp, err = client.PlaceBuyOrder(ctx, kisReq)
	case models.OrderTypeSell:
		kisResp, err = client.PlaceSellOrder(ctx, kisReq)
	default:
		return nil, fmt.Errorf("unknown order type: %s", req.OrderType)
	}

	status := models.OrderStatusPending
	kisOrderID := ""
	if err != nil {
		status = models.OrderStatusFailed
	} else {
		kisOrderID = kisResp.KISOrderID
	}

	// Persist order regardless of outcome for full audit trail.
	result, dbErr := db.ExecContext(ctx,
		`INSERT INTO orders (stock_code, order_type, qty, price, status, kis_order_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.StockCode, string(req.OrderType), req.Qty, req.Price, string(status), kisOrderID, time.Now().UTC(),
	)
	if dbErr != nil {
		return nil, fmt.Errorf("persist order: %w", dbErr)
	}

	orderID, _ := result.LastInsertId()

	if err != nil {
		return nil, fmt.Errorf("PlaceOrder KIS error: %w", err)
	}

	return &PlaceOrderResult{
		OrderID:    orderID,
		KISOrderID: kisOrderID,
		Status:     status,
	}, nil
}
