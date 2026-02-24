package agent

import (
	"context"
	"fmt"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

// GetOrderHistory returns KIS execution history and syncs status to the local DB.
func GetOrderHistory(ctx context.Context, client *kis.Client, db *database.DB) ([]map[string]any, error) {
	history, err := client.GetOrderHistory(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetOrderHistory: %w", err)
	}

	// Update local order statuses based on KIS execution data.
	for _, h := range history {
		kisOrderID, _ := h["odno"].(string)
		if kisOrderID == "" {
			continue
		}
		ccldQty, _ := h["tot_ccld_qty"].(string)
		if ccldQty != "" && ccldQty != "0" {
			_, _ = db.ExecContext(ctx,
				`UPDATE orders SET status = ? WHERE kis_order_id = ? AND status = ?`,
				string(models.OrderStatusFilled), kisOrderID, string(models.OrderStatusPending),
			)
		}
	}

	return history, nil
}

// GetLocalOrderHistory returns paginated orders from the local database.
func GetLocalOrderHistory(ctx context.Context, db *database.DB, limit, offset int) ([]models.Order, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, stock_code, order_type, qty, price, status, kis_order_id, created_at
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
		if err := rows.Scan(&o.ID, &o.StockCode, &o.OrderType, &o.Qty, &o.Price, &o.Status, &o.KISOrderID, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}
