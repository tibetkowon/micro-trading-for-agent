package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/logger"
	"github.com/micro-trading-for-agent/backend/internal/models"
	mqttpkg "github.com/micro-trading-for-agent/backend/internal/mqtt"
)

// MonitoredEntry holds a buy position being actively monitored.
type MonitoredEntry struct {
	StockCode   string
	StockName   string
	FilledPrice float64
	TargetPrice float64
	StopPrice   float64
	OrderID     int64
}

// Monitor watches registered positions for target/stop price hits and
// publishes MQTT alerts. It also handles end-of-day liquidation.
type Monitor struct {
	mu        sync.RWMutex
	positions map[string]*MonitoredEntry // stockCode → entry

	mqttPub   *mqttpkg.Publisher
	kisClient *kis.Client
	wsClient  *kis.WebSocketClient
	db        *database.DB
}

// New creates a Monitor. mqttPub may be nil (alerts are only logged).
func New(db *database.DB, kisClient *kis.Client, wsClient *kis.WebSocketClient, mqttPub *mqttpkg.Publisher) *Monitor {
	return &Monitor{
		positions: make(map[string]*MonitoredEntry),
		mqttPub:   mqttPub,
		kisClient: kisClient,
		wsClient:  wsClient,
		db:        db,
	}
}

// Register adds (or updates) a position to be monitored and persists it to DB.
// If wsClient is connected, it subscribes to real-time price updates.
func (m *Monitor) Register(ctx context.Context, pos MonitoredEntry) error {
	m.mu.Lock()
	m.positions[pos.StockCode] = &pos
	m.mu.Unlock()

	// Persist for server-restart recovery.
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO monitored_positions
		  (stock_code, stock_name, filled_price, target_price, stop_price, order_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(stock_code) DO UPDATE SET
		   stock_name=excluded.stock_name,
		   filled_price=excluded.filled_price,
		   target_price=excluded.target_price,
		   stop_price=excluded.stop_price,
		   order_id=excluded.order_id,
		   created_at=CURRENT_TIMESTAMP`,
		pos.StockCode, pos.StockName, pos.FilledPrice,
		pos.TargetPrice, pos.StopPrice, pos.OrderID,
	)
	if err != nil {
		return fmt.Errorf("persist monitored_position: %w", err)
	}

	// Subscribe to real-time price stream.
	if m.wsClient != nil {
		if err := m.wsClient.SubscribePrice(pos.StockCode); err != nil {
			logger.Error("ws subscribe price failed",
				map[string]any{"stock_code": pos.StockCode, "error": err.Error()})
		}
	}

	logger.Info("monitor: position registered",
		map[string]any{
			"stock_code":   pos.StockCode,
			"target_price": pos.TargetPrice,
			"stop_price":   pos.StopPrice,
		})
	return nil
}

// Remove removes a position from monitoring and deletes it from DB.
func (m *Monitor) Remove(ctx context.Context, stockCode string) {
	m.mu.Lock()
	delete(m.positions, stockCode)
	m.mu.Unlock()

	m.db.ExecContext(ctx, `DELETE FROM monitored_positions WHERE stock_code = ?`, stockCode)

	if m.wsClient != nil {
		m.wsClient.UnsubscribePrice(stockCode) //nolint:errcheck
	}

	logger.Info("monitor: position removed", map[string]any{"stock_code": stockCode})
}

// HandlePrice evaluates a price update against registered positions.
// Called by the WebSocket price event consumer goroutine.
func (m *Monitor) HandlePrice(stockCode string, price float64) {
	m.mu.RLock()
	pos, ok := m.positions[stockCode]
	m.mu.RUnlock()
	if !ok {
		return
	}

	switch {
	case price >= pos.TargetPrice:
		logger.Info("monitor: TARGET hit",
			map[string]any{"stock_code": stockCode, "price": price, "target": pos.TargetPrice})
		if m.mqttPub != nil {
			m.mqttPub.PublishAlert(mqttpkg.EventTargetHit,
				pos.StockCode, pos.StockName, price,
				pos.TargetPrice, pos.StopPrice, pos.FilledPrice)
		}
		// Remove from monitoring after alert; agent decides next action.
		m.Remove(context.Background(), stockCode)

	case price <= pos.StopPrice:
		logger.Info("monitor: STOP hit",
			map[string]any{"stock_code": stockCode, "price": price, "stop": pos.StopPrice})
		if m.mqttPub != nil {
			m.mqttPub.PublishAlert(mqttpkg.EventStopHit,
				pos.StockCode, pos.StockName, price,
				pos.TargetPrice, pos.StopPrice, pos.FilledPrice)
		}
		m.Remove(context.Background(), stockCode)
	}
}

// LiquidateAll places market sell orders for all monitored positions (15:15 장마감).
func (m *Monitor) LiquidateAll(ctx context.Context) {
	m.mu.RLock()
	codes := make([]string, 0, len(m.positions))
	for code := range m.positions {
		codes = append(codes, code)
	}
	m.mu.RUnlock()

	if len(codes) == 0 {
		return
	}

	logger.Info("monitor: liquidating all positions", map[string]any{"count": len(codes)})

	for _, code := range codes {
		m.mu.RLock()
		pos, ok := m.positions[code]
		m.mu.RUnlock()
		if !ok {
			continue
		}

		// Get holdings to find sellable qty.
		holdings, err := m.kisClient.GetHoldings(ctx)
		if err != nil {
			logger.Error("liquidate: GetHoldings failed",
				map[string]any{"stock_code": code, "error": err.Error()})
			continue
		}

		qty := 0
		for _, h := range holdings {
			if h.StockCode == code {
				fmt.Sscanf(h.HoldingQty, "%d", &qty)
				break
			}
		}
		if qty <= 0 {
			m.Remove(ctx, code)
			continue
		}

		_, err = m.kisClient.PlaceSellOrder(ctx, kis.OrderRequest{
			StockCode: code,
			OrderDivn: "01", // 시장가
			Qty:       fmt.Sprintf("%d", qty),
			Price:     "0",
		})
		if err != nil {
			logger.Error("liquidate: sell order failed",
				map[string]any{"stock_code": code, "error": err.Error()})
		} else {
			logger.Info("liquidate: sell order placed",
				map[string]any{"stock_code": code, "qty": qty})
		}

		if m.mqttPub != nil {
			m.mqttPub.PublishAlert(mqttpkg.EventLiquidation,
				pos.StockCode, pos.StockName, 0,
				pos.TargetPrice, pos.StopPrice, pos.FilledPrice)
		}

		m.Remove(ctx, code)
	}
}

// List returns a snapshot of all currently monitored positions.
func (m *Monitor) List() []models.MonitoredPosition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]models.MonitoredPosition, 0, len(m.positions))
	for _, pos := range m.positions {
		result = append(result, models.MonitoredPosition{
			StockCode:   pos.StockCode,
			StockName:   pos.StockName,
			FilledPrice: pos.FilledPrice,
			TargetPrice: pos.TargetPrice,
			StopPrice:   pos.StopPrice,
			OrderID:     pos.OrderID,
			CreatedAt:   time.Now(),
		})
	}
	return result
}

// Count returns the number of monitored positions.
func (m *Monitor) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.positions)
}

// LoadFromDB restores monitored positions from the database after a server restart.
func (m *Monitor) LoadFromDB(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx,
		`SELECT stock_code, stock_name, filled_price, target_price, stop_price, order_id
		 FROM monitored_positions`)
	if err != nil {
		return fmt.Errorf("load monitored_positions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var pos MonitoredEntry
		if err := rows.Scan(
			&pos.StockCode, &pos.StockName,
			&pos.FilledPrice, &pos.TargetPrice, &pos.StopPrice, &pos.OrderID,
		); err != nil {
			continue
		}
		m.mu.Lock()
		m.positions[pos.StockCode] = &pos
		m.mu.Unlock()

		if m.wsClient != nil {
			m.wsClient.SubscribePrice(pos.StockCode) //nolint:errcheck
		}
	}
	count := m.Count()
	if count > 0 {
		logger.Info("monitor: restored positions from DB", map[string]any{"count": count})
	}
	return nil
}

// StartPriceConsumer reads from wsClient.PriceCh and calls HandlePrice.
// Runs until ctx is cancelled.
func (m *Monitor) StartPriceConsumer(ctx context.Context) {
	if m.wsClient == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-m.wsClient.PriceCh:
			if !ok {
				return
			}
			m.HandlePrice(ev.StockCode, ev.Price)
		}
	}
}
