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

// IndicatorSnapshot holds key technical indicators for sell condition evaluation.
type IndicatorSnapshot struct {
	RSI14      float64
	MACDLine   float64
	MACDSignal float64
}

// MonitoredEntry holds a buy position being actively monitored.
type MonitoredEntry struct {
	StockCode   string
	StockName   string
	FilledPrice float64
	TargetPrice float64
	StopPrice   float64
	OrderID     int64
	SoldCh      chan<- string // optional: engine receives sold signal (may be nil)
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
// isTest=true: KIS 매도 주문을 건너뛰고 MQTT만 발행 (장 외 테스트용).
func (m *Monitor) HandlePrice(stockCode string, price float64, isTest bool) {
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
		sellQty := 0
		if !isTest {
			sellQty = m.executeSell(stockCode, pos)
		}
		if m.mqttPub != nil {
			m.mqttPub.PublishAlert(mqttpkg.EventTargetHit,
				pos.StockCode, pos.StockName, price,
				pos.TargetPrice, pos.StopPrice, pos.FilledPrice, sellQty, isTest)
		}
		m.Remove(context.Background(), stockCode)

	case price <= pos.StopPrice:
		logger.Info("monitor: STOP hit",
			map[string]any{"stock_code": stockCode, "price": price, "stop": pos.StopPrice})
		sellQty := 0
		if !isTest {
			sellQty = m.executeSell(stockCode, pos)
		}
		if m.mqttPub != nil {
			m.mqttPub.PublishAlert(mqttpkg.EventStopHit,
				pos.StockCode, pos.StockName, price,
				pos.TargetPrice, pos.StopPrice, pos.FilledPrice, sellQty, isTest)
		}
		m.Remove(context.Background(), stockCode)
	}
}

// executeSell places a market sell order for the given position and returns the qty sold.
// Returns 0 if holdings lookup fails, qty is 0, or sell order fails.
func (m *Monitor) executeSell(stockCode string, pos *MonitoredEntry) int {
	ctx := context.Background()

	holdings, err := m.kisClient.GetHoldings(ctx)
	if err != nil {
		logger.Error("auto-sell: GetHoldings failed",
			map[string]any{"stock_code": stockCode, "error": err.Error()})
		return 0
	}

	qty := 0
	for _, h := range holdings {
		if h.StockCode == stockCode {
			fmt.Sscanf(h.HoldingQty, "%d", &qty)
			break
		}
	}
	if qty <= 0 {
		logger.Info("auto-sell: no holdings found", map[string]any{"stock_code": stockCode})
		return 0
	}

	_, err = m.kisClient.PlaceSellOrder(ctx, kis.OrderRequest{
		StockCode: stockCode,
		OrderDivn: "01", // 시장가
		Qty:       fmt.Sprintf("%d", qty),
		Price:     "0",
	})
	if err != nil {
		logger.Error("auto-sell: PlaceSellOrder failed",
			map[string]any{"stock_code": stockCode, "qty": qty, "error": err.Error()})
		return 0
	}

	logger.Info("auto-sell: sell order placed",
		map[string]any{"stock_code": stockCode, "qty": qty, "filled_price": pos.FilledPrice})

	// Notify engine that this position was sold.
	if pos.SoldCh != nil {
		select {
		case pos.SoldCh <- stockCode:
		default:
		}
	}

	return qty
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
		var currentPrice float64
		for _, h := range holdings {
			if h.StockCode == code {
				fmt.Sscanf(h.HoldingQty, "%d", &qty)
				fmt.Sscanf(h.CurrentPrice, "%f", &currentPrice)
				break
			}
		}
		if qty <= 0 {
			m.Remove(ctx, code)
			continue
		}

		sellQty := 0
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
			sellQty = qty
			logger.Info("liquidate: sell order placed",
				map[string]any{"stock_code": code, "qty": qty})
		}

		if m.mqttPub != nil {
			// triggerPrice = 청산 시점 현재가 (시장가 매도의 근사 체결가)
			m.mqttPub.PublishAlert(mqttpkg.EventLiquidation,
				pos.StockCode, pos.StockName, currentPrice,
				pos.TargetPrice, pos.StopPrice, pos.FilledPrice, sellQty, false)
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

// StartIndicatorChecker periodically checks technical indicators for each monitored position
// and executes a sell if a configured condition is met.
// getInfoFn is a callback (injected to avoid circular imports) that returns the current indicators.
// conditions controls which checks are active and their priority order.
// Supported values: "rsi_overbought", "macd_bearish" (target_pct/stop_pct are handled by HandlePrice).
func (m *Monitor) StartIndicatorChecker(
	ctx context.Context,
	intervalMin int,
	conditions []string,
	rsiThreshold float64,
	macdBearish bool,
	getInfoFn func(ctx context.Context, code string) (*IndicatorSnapshot, error),
) {
	if intervalMin <= 0 {
		intervalMin = 5
	}
	ticker := time.NewTicker(time.Duration(intervalMin) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkIndicators(ctx, conditions, rsiThreshold, macdBearish, getInfoFn)
		}
	}
}

func (m *Monitor) checkIndicators(
	ctx context.Context,
	conditions []string,
	rsiThreshold float64,
	macdBearish bool,
	getInfoFn func(ctx context.Context, code string) (*IndicatorSnapshot, error),
) {
	m.mu.RLock()
	codes := make([]string, 0, len(m.positions))
	for code := range m.positions {
		codes = append(codes, code)
	}
	m.mu.RUnlock()

	for _, code := range codes {
		snap, err := getInfoFn(ctx, code)
		if err != nil {
			logger.Error("indicator check: getInfoFn failed",
				map[string]any{"stock_code": code, "error": err.Error()})
			continue
		}

		m.mu.RLock()
		pos, ok := m.positions[code]
		m.mu.RUnlock()
		if !ok {
			continue
		}

		triggered := false
		triggerReason := ""

		for _, cond := range conditions {
			switch cond {
			case "rsi_overbought":
				if snap.RSI14 > 0 && rsiThreshold > 0 && snap.RSI14 >= rsiThreshold {
					triggered = true
					triggerReason = fmt.Sprintf("RSI %.2f >= threshold %.2f", snap.RSI14, rsiThreshold)
				}
			case "macd_bearish":
				if macdBearish && snap.MACDLine != 0 && snap.MACDLine < snap.MACDSignal {
					triggered = true
					triggerReason = fmt.Sprintf("MACD bearish crossover: line=%.4f signal=%.4f", snap.MACDLine, snap.MACDSignal)
				}
			}
			if triggered {
				break
			}
		}

		if !triggered {
			continue
		}

		logger.Info("indicator check: sell condition triggered",
			map[string]any{"stock_code": code, "reason": triggerReason})
		sellQty := m.executeSell(code, pos)
		if m.mqttPub != nil && sellQty > 0 {
			m.mqttPub.PublishAlert(mqttpkg.EventTargetHit,
				pos.StockCode, pos.StockName, 0,
				pos.TargetPrice, pos.StopPrice, pos.FilledPrice, sellQty, false)
		}
		m.Remove(ctx, code)
	}
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
			m.HandlePrice(ev.StockCode, ev.Price, false)
		}
	}
}
