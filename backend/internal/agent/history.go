package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/logger"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

// GetOrderHistory returns KIS execution history and syncs status, stock name, and filled price to the local DB.
// For KIS orders not yet in the local DB (manually placed via KIS app/web), a new record is inserted with source=MANUAL.
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

		stockCode, _ := h["pdno"].(string)
		stockName, _ := h["prdt_name"].(string)
		ccldQty, _ := h["tot_ccld_qty"].(string)
		ordQty, _ := h["ord_qty"].(string)
		avgPrvs, _ := h["avg_prvs"].(string)   // 평균체결가
		ordUnpr, _ := h["ord_unpr"].(string)   // 주문단가
		sllBuy, _ := h["sll_buy_dvsn_cd"].(string) // 01=매도 02=매수
		ordDt, _ := h["ord_dt"].(string)        // 주문일자 YYYYMMDD
		ordTmd, _ := h["ord_tmd"].(string)      // 주문시각 HHMMSS
		cancYn, _ := h["cncl_yn"].(string)      // 취소여부 Y/N

		// 로컬 DB에 이미 존재하는지 확인
		var existing int
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM orders WHERE CAST(kis_order_id AS INTEGER) = CAST(? AS INTEGER)`,
			kisOrderID,
		).Scan(&existing)

		if existing > 0 {
			// --- 기존 레코드: 상태 / 종목명 / 체결가 업데이트 ---
			if ccldQty == "" || ccldQty == "0" {
				// 미체결 — 종목명만 업데이트
				if stockName != "" {
					_, _ = db.ExecContext(ctx,
						`UPDATE orders SET stock_name = ? WHERE CAST(kis_order_id AS INTEGER) = CAST(? AS INTEGER) AND stock_name = ''`,
						stockName, kisOrderID,
					)
				}
				continue
			}

			newStatus := models.OrderStatusFilled
			if ordQty != "" && ordQty != ccldQty {
				newStatus = models.OrderStatusPartiallyFilled
			}
			filledPrice, _ := strconv.ParseFloat(avgPrvs, 64)

			_, _ = db.ExecContext(ctx,
				`UPDATE orders SET status = ?, filled_price = ?, stock_name = ?
				 WHERE CAST(kis_order_id AS INTEGER) = CAST(? AS INTEGER) AND status IN ('PENDING','PARTIALLY_FILLED')`,
				string(newStatus), filledPrice, stockName, kisOrderID,
			)
			continue
		}

		// --- 신규 레코드: 수동 거래 감지 → INSERT ---
		if stockCode == "" {
			continue
		}

		// 주문 유형
		orderType := models.OrderTypeBuy
		if sllBuy == "01" {
			orderType = models.OrderTypeSell
		}

		// 상태 결정
		var status models.OrderStatus
		switch {
		case cancYn == "Y":
			status = models.OrderStatusCancelled
		case ccldQty == "" || ccldQty == "0":
			status = models.OrderStatusPending
		case ordQty != "" && ordQty == ccldQty:
			status = models.OrderStatusFilled
		default:
			status = models.OrderStatusPartiallyFilled
		}

		price, _ := strconv.ParseFloat(ordUnpr, 64)
		filledPrice, _ := strconv.ParseFloat(avgPrvs, 64)
		qty, _ := strconv.Atoi(ordQty)
		if qty <= 0 {
			continue
		}

		// 실제 주문 시각 파싱 (ord_dt="20060102", ord_tmd="150405")
		orderedAt := time.Now()
		if ordDt != "" && ordTmd != "" {
			if t, err := time.ParseInLocation("20060102150405", ordDt+ordTmd, time.Local); err == nil {
				orderedAt = t
			}
		} else if ordDt != "" {
			if t, err := time.ParseInLocation("20060102", ordDt, time.Local); err == nil {
				orderedAt = t
			}
		}

		_, err := db.ExecContext(ctx,
			`INSERT INTO orders (stock_code, stock_name, order_type, qty, price, filled_price, status, kis_order_id, source, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'MANUAL', ?)`,
			stockCode, stockName, string(orderType), qty, price, filledPrice,
			string(status), kisOrderID, orderedAt,
		)
		if err != nil {
			logger.Warn("manual trade insert failed", map[string]any{
				"kis_order_id": kisOrderID,
				"error":        err.Error(),
			})
		} else {
			logger.Info("manual trade imported", map[string]any{
				"kis_order_id": kisOrderID,
				"stock_code":   stockCode,
				"order_type":   string(orderType),
				"status":       string(status),
			})
		}
	}

	return history, nil
}

// StartOrderSyncScheduler runs GetOrderHistory every interval in a background goroutine.
// It skips the sync if there are no PENDING/PARTIALLY_FILLED orders to avoid unnecessary KIS API calls.
// The goroutine stops when ctx is cancelled (aligned with server graceful shutdown).
func StartOrderSyncScheduler(ctx context.Context, client *kis.Client, db *database.DB, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 미체결 주문이 없어도 수동 거래 감지를 위해 항상 동기화 실행
				if _, err := GetOrderHistory(ctx, client, db); err != nil {
					logger.Warn("order sync failed", map[string]any{"error": err.Error()})
				} else {
					logger.Info("order sync completed", nil)
				}
			}
		}
	}()
}

// GetLocalOrderHistory returns paginated orders from the local database,
// sorted by actual order time (created_at DESC) so manual and agent orders appear in correct chronological order.
func GetLocalOrderHistory(ctx context.Context, db *database.DB, limit, offset int) ([]models.Order, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, stock_code, stock_name, order_type, qty, price, filled_price, status, kis_order_id, source, created_at
		 FROM orders ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.StockCode, &o.StockName, &o.OrderType, &o.Qty, &o.Price, &o.FilledPrice, &o.Status, &o.KISOrderID, &o.Source, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}
