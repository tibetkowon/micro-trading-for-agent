package trader

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/agent"
	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/logger"
	"github.com/micro-trading-for-agent/backend/internal/models"
	"github.com/micro-trading-for-agent/backend/internal/monitor"
	mqttpkg "github.com/micro-trading-for-agent/backend/internal/mqtt"
)

// EngineState represents the current phase of the trading engine.
type EngineState string

const (
	StateIdle        EngineState = "IDLE"
	StateSelecting   EngineState = "SELECTING"
	StateOrdering    EngineState = "ORDERING"
	StateWaitingFill EngineState = "WAITING_FILL"
	StateMonitoring  EngineState = "MONITORING"
)

// Engine runs autonomous trading cycles: select → order → monitor → repeat.
type Engine struct {
	db        *database.DB
	kisClient *kis.Client
	wsClient  *kis.WebSocketClient
	mon       *monitor.Monitor
	mqttPub   *mqttpkg.Publisher
	claude    *ClaudeClient

	mu     sync.RWMutex
	state  EngineState
	soldCh chan string // receives stock_code when monitor executes a sell
	stopCh chan struct{}
}

// NewEngine creates a new Engine with all required dependencies.
// claude may be nil if ANTHROPIC_API_KEY is not configured (engine will log an error and sleep).
func NewEngine(
	db *database.DB,
	kisClient *kis.Client,
	wsClient *kis.WebSocketClient,
	mon *monitor.Monitor,
	mqttPub *mqttpkg.Publisher,
	claude *ClaudeClient,
) *Engine {
	return &Engine{
		db:        db,
		kisClient: kisClient,
		wsClient:  wsClient,
		mon:       mon,
		mqttPub:   mqttPub,
		claude:    claude,
		state:     StateIdle,
		soldCh:    make(chan string, 16),
		stopCh:    make(chan struct{}),
	}
}

// GetState returns the current engine state (thread-safe).
func (e *Engine) GetState() EngineState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

func (e *Engine) setState(s EngineState) {
	e.mu.Lock()
	e.state = s
	e.mu.Unlock()
	logger.Info("engine: state changed", map[string]any{"state": string(s)})
}

// Start launches the trading cycle goroutine and returns a stop function.
func (e *Engine) Start(ctx context.Context) (stop func()) {
	cycleCtx, cancel := context.WithCancel(ctx)
	e.stopCh = make(chan struct{})

	go e.runCycle(cycleCtx)

	return func() {
		cancel()
		e.setState(StateIdle)
		logger.Info("engine: stopped", nil)
	}
}

// SoldCh returns the channel that should be sent to when a monitored position is sold.
// Pass this as SoldCh when registering MonitoredEntry objects.
func (e *Engine) SoldCh() chan<- string {
	return e.soldCh
}

// retryBackoff returns wait duration based on consecutive failure count.
// 1st: 30s, 2nd: 1m, 3rd: 3m, 4th+: 5m
func retryBackoff(failures int) time.Duration {
	switch failures {
	case 1:
		return 30 * time.Second
	case 2:
		return 1 * time.Minute
	case 3:
		return 3 * time.Minute
	default:
		return 5 * time.Minute
	}
}

func (e *Engine) runCycle(ctx context.Context) {
	e.setState(StateMonitoring)

	consecutiveFailures := 0

	for {
		select {
		case <-ctx.Done():
			e.setState(StateIdle)
			return
		default:
		}

		settings, err := e.db.GetTradingSettings(ctx)
		if err != nil {
			logger.Error("engine: GetTradingSettings failed", map[string]any{"error": err.Error()})
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
			continue
		}

		currentCount := e.mon.Count()
		if currentCount >= settings.MaxPositions {
			consecutiveFailures = 0 // 포지션 보유 중 = 정상 상태
			select {
			case <-ctx.Done():
				e.setState(StateIdle)
				return
			case code := <-e.soldCh:
				logger.Info("engine: sold signal received, resuming cycle",
					map[string]any{"stock_code": code})
			case <-time.After(30 * time.Second):
			}
			continue
		}

		if e.claude == nil {
			logger.Error("engine: claude client not configured (ANTHROPIC_API_KEY missing)", nil)
			select {
			case <-ctx.Done():
				return
			case <-time.After(60 * time.Second):
			}
			continue
		}

		if err := e.selectAndBuy(ctx, settings); err != nil {
			consecutiveFailures++
			wait := retryBackoff(consecutiveFailures)
			logger.Error("engine: selectAndBuy failed",
				map[string]any{"error": err.Error(), "failures": consecutiveFailures, "retry_in": wait.String()})
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		} else {
			consecutiveFailures = 0
		}
	}
}

func (e *Engine) selectAndBuy(ctx context.Context, settings database.TradingSettings) error {
	e.setState(StateSelecting)

	// Build today's exclusion list from DB orders.
	excludedCodes := e.getTodayTradedCodes(ctx)

	// Fetch rankings based on configured types.
	rankings, err := e.getRankings(ctx, settings)
	if err != nil {
		e.setState(StateMonitoring)
		return fmt.Errorf("getRankings: %w", err)
	}
	if len(rankings) == 0 {
		e.setState(StateMonitoring)
		return fmt.Errorf("no ranking results after intersection filter")
	}

	// Enrich each candidate with technical indicators (MA5/MA20/RSI/MACD).
	for i, r := range rankings {
		info, err := agent.GetStockInfo(ctx, e.kisClient, r.StockCode)
		if err != nil {
			logger.Warn("engine: GetStockInfo failed, skipping indicators",
				map[string]any{"stock_code": r.StockCode, "error": err.Error()})
			continue
		}
		rankings[i].MA5 = info.MA5
		rankings[i].MA20 = info.MA20
		rankings[i].RSI14 = info.RSI14
		rankings[i].MACDLine = info.MACDLine
		rankings[i].MACDSignal = info.MACDSignal
	}

	// Get available cash.
	summary, err := e.kisClient.GetInquireBalance(ctx)
	if err != nil {
		e.setState(StateMonitoring)
		return fmt.Errorf("GetInquireBalance: %w", err)
	}
	availableCash, _ := strconv.ParseFloat(summary.DepositAmt, 64)
	if availableCash <= 0 {
		e.setState(StateMonitoring)
		return fmt.Errorf("no available cash")
	}

	// Ask Claude to rank all viable candidates (single API call).
	candidates, err := e.claude.SelectStocks(ctx, rankings, availableCash, excludedCodes)
	if err != nil {
		e.setState(StateMonitoring)
		return fmt.Errorf("SelectStocks: %w", err)
	}
	logger.Info("engine: Claude ranked candidates",
		map[string]any{"count": len(candidates)})

	// Try candidates in order until one succeeds.
	var (
		stockCode   string
		filledPrice float64
		filledQty   int
		result      *agent.PlaceOrderResult
	)
	for i, candidate := range candidates {
		code := candidate.StockCode
		logger.Info("engine: trying candidate",
			map[string]any{"rank": i + 1, "stock_code": code, "reason": candidate.Reason})

		e.setState(StateOrdering)

		feasibility, ferr := agent.CheckOrderFeasibility(ctx, e.kisClient, code)
		if ferr != nil || feasibility.OrderableQty <= 0 {
			logger.Warn("engine: candidate not orderable, skipping",
				map[string]any{"stock_code": code})
			continue
		}

		qty := int(float64(feasibility.OrderableQty) * settings.OrderAmountPct / 100)
		if qty <= 0 {
			qty = 1
		}

		res, perr := agent.PlaceOrder(ctx, e.kisClient, e.db, agent.PlaceOrderRequest{
			StockCode: code,
			OrderType: models.OrderTypeBuy,
			Qty:       qty,
			Price:     0,
			OrderDivn: "01",
			TargetPct: settings.TakeProfitPct,
			StopPct:   settings.StopLossPct,
		})
		if perr != nil {
			logger.Warn("engine: PlaceOrder failed, skipping",
				map[string]any{"stock_code": code, "error": perr.Error()})
			continue
		}

		e.setState(StateWaitingFill)

		fillCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		fp, fq, filled := e.waitForFill(fillCtx, res.KISOrderID)
		cancel()

		if !filled {
			if _, cancelErr := agent.CancelOrder(ctx, e.kisClient, e.db, res.OrderID); cancelErr != nil {
				logger.Warn("engine: cancel order failed after fill timeout",
					map[string]any{"order_id": res.OrderID, "error": cancelErr.Error()})
			}
			logger.Warn("engine: fill timeout, trying next candidate",
				map[string]any{"stock_code": code})
			continue
		}

		// Success.
		stockCode = code
		filledPrice = fp
		filledQty = fq
		result = res
		break
	}

	if result == nil {
		e.setState(StateMonitoring)
		return fmt.Errorf("all %d candidates failed", len(candidates))
	}

	logger.Info("engine: order filled",
		map[string]any{
			"stock_code":   stockCode,
			"filled_price": filledPrice,
			"filled_qty":   filledQty,
		})

	// Update DB with fill.
	e.db.ExecContext(ctx, //nolint:errcheck
		`UPDATE orders SET filled_price = ?, status = ? WHERE id = ?`,
		filledPrice, string(models.OrderStatusFilled), result.OrderID)

	// Determine stock name from ranking list.
	stockName := stockCode
	for _, r := range rankings {
		if r.StockCode == stockCode {
			stockName = r.StockName
			break
		}
	}

	// Register with monitor.
	entry := monitor.MonitoredEntry{
		StockCode:   stockCode,
		StockName:   stockName,
		FilledPrice: filledPrice,
		TargetPrice: filledPrice * (1 + settings.TakeProfitPct/100),
		StopPrice:   filledPrice * (1 - settings.StopLossPct/100),
		OrderID:     result.OrderID,
		SoldCh:      e.soldCh,
	}
	if regErr := e.mon.Register(ctx, entry); regErr != nil {
		logger.Error("engine: Register position failed",
			map[string]any{"error": regErr.Error()})
	}

	e.setState(StateMonitoring)
	return nil
}

// waitForFill drains ExecCh looking for a fill event matching kisOrderID.
// Returns (filledPrice, filledQty, true) on fill, or (0, 0, false) on timeout.
func (e *Engine) waitForFill(ctx context.Context, kisOrderID string) (float64, int, bool) {
	if e.wsClient == nil {
		// No WebSocket — cannot wait for fill.
		return 0, 0, false
	}

	for {
		select {
		case <-ctx.Done():
			return 0, 0, false
		case ev, ok := <-e.wsClient.ExecCh:
			if !ok {
				return 0, 0, false
			}
			// Match: same KIS order ID, filled (CntgYN=="2"), buy side (SellBuyDiv=="02").
			if ev.KISOrderID == kisOrderID && ev.CntgYN == "2" && ev.SellBuyDiv == "02" {
				return ev.FilledPrice, ev.FilledQty, true
			}
		}
	}
}

// getTodayTradedCodes returns stock codes that have been traded today from DB.
func (e *Engine) getTodayTradedCodes(ctx context.Context) []string {
	kst, _ := time.LoadLocation("Asia/Seoul")
	today := time.Now().In(kst).Format("2006-01-02")

	rows, err := e.db.QueryContext(ctx,
		`SELECT DISTINCT stock_code FROM orders
		 WHERE date(created_at) = date(?) AND source = 'AGENT'`, today)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err == nil {
			codes = append(codes, code)
		}
	}
	return codes
}

// getRankings calls each configured ranking API, applies per-type filters,
// then returns only stocks that passed ALL enabled ranking types (AND logic).
// Fields from each ranking type are merged into a single RankItem per stock.
func (e *Engine) getRankings(ctx context.Context, settings database.TradingSettings) ([]RankItem, error) {
	excludeCls := e.db.GetSetting(ctx, "ranking_excl_cls")
	priceMin := settings.RankingPriceMin
	priceMax := settings.RankingPriceMax

	// byType holds filtered results per ranking type: stockCode → RankItem
	byType := make(map[string]map[string]RankItem) // rankingType → code → item

	for _, rt := range settings.RankingTypes {
		byType[rt] = make(map[string]RankItem)

		switch rt {
		case "volume":
			items, err := e.kisClient.GetVolumeRank(ctx, "J", "0", priceMin, priceMax, excludeCls)
			if err != nil {
				logger.Warn("engine: GetVolumeRank failed", map[string]any{"error": err.Error()})
				continue
			}
			for _, item := range items {
				if settings.RankingVolumeMinIncrRate > 0 {
					rate, _ := strconv.ParseFloat(item.VolIncrRate, 64)
					if rate < settings.RankingVolumeMinIncrRate {
						continue
					}
				}
				byType[rt][item.StockCode] = RankItem{
					DataRank: item.DataRank, StockCode: item.StockCode,
					StockName: item.StockName, CurrentPrice: item.CurrentPrice,
					Volume: item.Volume, VolIncrRate: item.VolIncrRate,
				}
			}

		case "strength":
			items, err := e.kisClient.GetStrengthRank(ctx, "0000", priceMin, priceMax, excludeCls)
			if err != nil {
				logger.Warn("engine: GetStrengthRank failed", map[string]any{"error": err.Error()})
				continue
			}
			for _, item := range items {
				if settings.RankingStrengthMin > 0 {
					str, _ := strconv.ParseFloat(item.Strength, 64)
					if str < settings.RankingStrengthMin {
						continue
					}
				}
				byType[rt][item.StockCode] = RankItem{
					DataRank: item.DataRank, StockCode: item.StockCode,
					StockName: item.StockName, CurrentPrice: item.CurrentPrice,
					Volume: item.Volume, Strength: item.Strength,
				}
			}

		case "exec_count":
			items, err := e.kisClient.GetExecCountRank(ctx, "0000", "0", priceMin, priceMax, excludeCls)
			if err != nil {
				logger.Warn("engine: GetExecCountRank failed", map[string]any{"error": err.Error()})
				continue
			}
			for _, item := range items {
				if settings.RankingExecCountNetBuyOnly {
					netBuy, _ := strconv.ParseFloat(item.NetBuyQty, 64)
					if netBuy <= 0 {
						continue
					}
				}
				byType[rt][item.StockCode] = RankItem{
					DataRank: item.DataRank, StockCode: item.StockCode,
					StockName: item.StockName, CurrentPrice: item.CurrentPrice,
					Volume: item.Volume, NetBuyQty: item.NetBuyQty,
				}
			}

		case "disparity":
			items, err := e.kisClient.GetDisparityRank(ctx, "0000", "20", "0", priceMin, priceMax, excludeCls)
			if err != nil {
				logger.Warn("engine: GetDisparityRank failed", map[string]any{"error": err.Error()})
				continue
			}
			for _, item := range items {
				if settings.RankingDisparityD20Min > 0 || settings.RankingDisparityD20Max > 0 {
					d20, _ := strconv.ParseFloat(item.D20, 64)
					if settings.RankingDisparityD20Min > 0 && d20 < settings.RankingDisparityD20Min {
						continue
					}
					if settings.RankingDisparityD20Max > 0 && d20 > settings.RankingDisparityD20Max {
						continue
					}
				}
				byType[rt][item.StockCode] = RankItem{
					DataRank: item.DataRank, StockCode: item.StockCode,
					StockName: item.StockName, CurrentPrice: item.CurrentPrice,
					Volume: item.Volume, DisparityD20: item.D20,
				}
			}
		}
	}

	if len(byType) == 0 {
		return nil, fmt.Errorf("no ranking types configured")
	}

	// AND intersection: keep only stocks present in every enabled ranking type.
	// Use the first type as seed, then filter against the rest.
	var seedType string
	for k := range byType {
		seedType = k
		break
	}

	var result []RankItem
	for code, base := range byType[seedType] {
		inAll := true
		for rt, m := range byType {
			if rt == seedType {
				continue
			}
			if _, ok := m[code]; !ok {
				inAll = false
				break
			}
		}
		if !inAll {
			continue
		}

		// Merge fields from all ranking types into one RankItem.
		merged := base
		merged.RankingType = strings.Join(settings.RankingTypes, "+")
		for rt, m := range byType {
			if rt == seedType {
				continue
			}
			other := m[code]
			if other.VolIncrRate != "" {
				merged.VolIncrRate = other.VolIncrRate
			}
			if other.Strength != "" {
				merged.Strength = other.Strength
			}
			if other.NetBuyQty != "" {
				merged.NetBuyQty = other.NetBuyQty
			}
			if other.DisparityD20 != "" {
				merged.DisparityD20 = other.DisparityD20
			}
		}
		result = append(result, merged)
	}

	logger.Info("engine: rankings intersection", map[string]any{
		"types": settings.RankingTypes,
		"count": len(result),
	})

	return result, nil
}

// tradeRow holds a matched buy+sell pair for report generation.
type tradeRow struct {
	StockName string
	BuyPrice  float64
	SellPrice float64
	Qty       int
	PnL       float64
	PnLPct    float64
}

// GenerateDailyReport builds a markdown report: server renders the table,
// Claude writes the analysis section.
func (e *Engine) GenerateDailyReport(ctx context.Context) (string, error) {
	if e.claude == nil {
		return "", fmt.Errorf("claude client not configured")
	}

	kst, _ := time.LoadLocation("Asia/Seoul")
	today := time.Now().In(kst).Format("2006-01-02")

	// Load today's FILLED orders from DB.
	rows, err := e.db.QueryContext(ctx,
		`SELECT stock_code, stock_name, order_type, qty, filled_price
		 FROM orders
		 WHERE date(created_at) = date(?) AND source = 'AGENT' AND status = 'FILLED'
		 ORDER BY id`, today)
	if err != nil {
		return "", fmt.Errorf("load today's orders: %w", err)
	}
	defer rows.Close()

	type orderRow struct {
		Code        string
		Name        string
		Type        string
		Qty         int
		FilledPrice float64
	}
	var orders []orderRow
	for rows.Next() {
		var o orderRow
		if err := rows.Scan(&o.Code, &o.Name, &o.Type, &o.Qty, &o.FilledPrice); err == nil {
			orders = append(orders, o)
		}
	}

	// Match BUY → SELL pairs per stock code (FIFO).
	buyMap := map[string][]orderRow{}
	var trades []tradeRow
	for _, o := range orders {
		if o.Type == "BUY" {
			buyMap[o.Code] = append(buyMap[o.Code], o)
		} else if o.Type == "SELL" {
			if buys := buyMap[o.Code]; len(buys) > 0 {
				buy := buys[0]
				buyMap[o.Code] = buys[1:]
				pnl := (o.FilledPrice - buy.FilledPrice) * float64(o.Qty)
				pnlPct := (o.FilledPrice - buy.FilledPrice) / buy.FilledPrice * 100
				trades = append(trades, tradeRow{
					StockName: buy.Name,
					BuyPrice:  buy.FilledPrice,
					SellPrice: o.FilledPrice,
					Qty:       o.Qty,
					PnL:       pnl,
					PnLPct:    pnlPct,
				})
			}
		}
	}

	// Build markdown table.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s 트레이딩 리포트\n\n", today))
	sb.WriteString("## 거래 결과\n\n")
	if len(trades) == 0 {
		sb.WriteString("오늘 완결된 거래가 없습니다.\n")
	} else {
		sb.WriteString("| 종목 | 매수가 | 매도가 | 수량 | 손익 | 수익률 |\n")
		sb.WriteString("|------|--------|--------|------|------|--------|\n")
		totalPnL := 0.0
		winCount, lossCount := 0, 0
		for _, t := range trades {
			sign := "+"
			if t.PnL < 0 {
				sign = ""
				lossCount++
			} else {
				winCount++
			}
			sb.WriteString(fmt.Sprintf("| %s | %.0f | %.0f | %d | %s%.0f원 | %s%.1f%% |\n",
				t.StockName, t.BuyPrice, t.SellPrice, t.Qty, sign, t.PnL, sign, t.PnLPct))
			totalPnL += t.PnL
		}
		sign := "+"
		if totalPnL < 0 {
			sign = ""
		}
		sb.WriteString(fmt.Sprintf("\n**총 실현 손익: %s%.0f원 | 승률: %d/%d**\n",
			sign, totalPnL, winCount, len(trades)))

		// Get account balance for Claude summary.
		totalEval, withdrawable := 0.0, 0.0
		if summary, balErr := e.kisClient.GetInquireBalance(ctx); balErr == nil {
			totalEval, _ = strconv.ParseFloat(summary.TotalEval, 64)
			withdrawable, _ = strconv.ParseFloat(summary.DepositAmt, 64)
		}

		// Claude writes the analysis section only.
		analysis, claudeErr := e.claude.GenerateReport(ctx, ReportSummary{
			Date:         today,
			TotalPnL:     totalPnL,
			WinCount:     winCount,
			LossCount:    lossCount,
			TotalEval:    totalEval,
			Withdrawable: withdrawable,
		})
		if claudeErr != nil {
			logger.Warn("report: claude analysis failed", map[string]any{"err": claudeErr.Error()})
			analysis = "(AI 분석 생성 실패)"
		}
		sb.WriteString("\n## AI 분석\n\n")
		sb.WriteString(analysis)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
