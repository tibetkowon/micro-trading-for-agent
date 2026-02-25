package agent

import (
	"context"
	"fmt"
	"strconv"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

// GetOrderHistory returns KIS execution history and syncs status, stock name, and filled price to the local DB.
func GetOrderHistory(ctx context.Context, client *kis.Client, db *database.DB) ([]map[string]any, error) {
	history, err := client.GetOrderHistory(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetOrderHistory: %w", err)
	}

	for _, h := range history {
		kisOrderID, _ := h["odno"].(string)
		if kisOrderID == "" {
			continue
		}

		stockName, _ := h["prdt_name"].(string)
		ccldQty, _ := h["tot_ccld_qty"].(string)
		ordQty, _ := h["ord_qty"].(string)
		avgPrvs, _ := h["avg_prvs"].(string) // 평균체결가

		if ccldQty == "" || ccldQty == "0" {
			// 미체결 — 종목명만 업데이트
			if stockName != "" {
				_, _ = db.ExecContext(ctx,
					`UPDATE orders SET stock_name = ? WHERE kis_order_id = ? AND stock_name = ''`,
					stockName, kisOrderID,
				)
			}
			continue
		}

		// 체결 상태 판별: 부분체결 vs 완전체결
		newStatus := models.OrderStatusFilled
		if ordQty != "" && ordQty != ccldQty {
			newStatus = models.OrderStatusPartiallyFilled
		}

		filledPrice, _ := strconv.ParseFloat(avgPrvs, 64)

		_, _ = db.ExecContext(ctx,
			`UPDATE orders SET status = ?, filled_price = ?, stock_name = ?
			 WHERE kis_order_id = ? AND status IN ('PENDING','PARTIALLY_FILLED')`,
			string(newStatus), filledPrice, stockName, kisOrderID,
		)
	}

	return history, nil
}

// GetLocalOrderHistory returns paginated orders from the local database.
func GetLocalOrderHistory(ctx context.Context, db *database.DB, limit, offset int) ([]models.Order, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, stock_code, stock_name, order_type, qty, price, filled_price, status, kis_order_id, created_at
		 FROM orders ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.StockCode, &o.StockName, &o.OrderType, &o.Qty, &o.Price, &o.FilledPrice, &o.Status, &o.KISOrderID, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}
