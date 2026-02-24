package agent

import (
	"context"
	"fmt"
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
