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

// CancelOrderResult is returned after a successful order cancellation.
type CancelOrderResult struct {
	OrderID    int64
	KISOrderID string // 취소 주문번호 (KIS에서 새로 채번된 번호)
}

// PlaceOrderRequest contains the parameters for a new order.
type PlaceOrderRequest struct {
	StockCode string
	OrderType models.OrderType
	Qty       int
	Price     float64 // 0 for market order
	OrderDivn string  // "00"=지정가, "01"=시장가
	TargetPct float64 // 목표 수익률 (%) — 0이면 모니터링 미등록
	StopPct   float64 // 손절 비율 (%) — 0이면 모니터링 미등록
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
		`INSERT INTO orders (stock_code, order_type, qty, price, status, kis_order_id, target_pct, stop_pct, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.StockCode, string(req.OrderType), req.Qty, req.Price, string(status), kisOrderID,
		req.TargetPct, req.StopPct, time.Now().UTC(),
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

// CancelOrder cancels a pending KIS order identified by its local DB id.
//
// Flow:
//  1. Fetch local order to get kis_order_id and current status.
//  2. Call TTTC0084R to get the cancellable order list and verify psbl_qty > 0.
//  3. Call TTTC0013U to submit the cancellation.
//  4. Update local DB status to CANCELLED.
func CancelOrder(ctx context.Context, client *kis.Client, db *database.DB, orderID int64) (*CancelOrderResult, error) {
	// 1. Look up local order
	var kisOrderID, status string
	err := db.QueryRowContext(ctx,
		`SELECT kis_order_id, status FROM orders WHERE id = ?`, orderID,
	).Scan(&kisOrderID, &status)
	if err != nil {
		return nil, fmt.Errorf("order %d not found: %w", orderID, err)
	}
	if kisOrderID == "" {
		return nil, fmt.Errorf("order %d has no KIS order ID (may have failed on submission)", orderID)
	}
	if status == string(models.OrderStatusCancelled) {
		return nil, fmt.Errorf("order %d is already cancelled", orderID)
	}
	if status == string(models.OrderStatusFilled) {
		return nil, fmt.Errorf("order %d is already filled and cannot be cancelled", orderID)
	}

	// 2. Get cancellable orders from KIS to find krxOrgNo and psbl_qty
	cancellable, err := client.GetCancellableOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetCancellableOrders: %w", err)
	}

	var found *kis.CancellableOrderItem
	for i := range cancellable {
		if cancellable[i].Odno == kisOrderID {
			found = &cancellable[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("order %s not found in KIS cancellable list (may be already filled or settled)", kisOrderID)
	}

	psblQty, _ := strconv.Atoi(found.PsblQty)
	if psblQty <= 0 {
		return nil, fmt.Errorf("order %s has no cancellable quantity (psbl_qty=%s)", kisOrderID, found.PsblQty)
	}

	// 3. Submit cancellation to KIS
	cancelResp, err := client.CancelKISOrder(ctx, found.OrdGnoBrno, kisOrderID, found.OrdDvsnCd, found.OrdUnpr)
	if err != nil {
		return nil, fmt.Errorf("CancelKISOrder: %w", err)
	}

	// 4. Update local DB status
	_, dbErr := db.ExecContext(ctx,
		`UPDATE orders SET status = ? WHERE id = ?`,
		string(models.OrderStatusCancelled), orderID,
	)
	if dbErr != nil {
		// KIS 취소는 성공했지만 DB 갱신 실패 — 오류를 반환하되 취소 결과는 포함
		return &CancelOrderResult{OrderID: orderID, KISOrderID: cancelResp.KISOrderID},
			fmt.Errorf("KIS cancel succeeded but DB update failed: %w", dbErr)
	}

	return &CancelOrderResult{
		OrderID:    orderID,
		KISOrderID: cancelResp.KISOrderID,
	}, nil
}
