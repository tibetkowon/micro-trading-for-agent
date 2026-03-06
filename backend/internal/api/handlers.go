package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/micro-trading-for-agent/backend/internal/agent"
	"github.com/micro-trading-for-agent/backend/internal/config"
	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/models"
	"github.com/micro-trading-for-agent/backend/internal/monitor"
	"github.com/micro-trading-for-agent/backend/internal/trader"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	db           *database.DB
	client       *kis.Client
	tokenManager *kis.TokenManager
	cfg          *config.Config
	monitor      *monitor.Monitor
	wsClient     *kis.WebSocketClient
	engine       *trader.Engine
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(db *database.DB, client *kis.Client, tokenManager *kis.TokenManager,
	cfg *config.Config, mon *monitor.Monitor, wsClient *kis.WebSocketClient) *Handler {
	return &Handler{
		db:           db,
		client:       client,
		tokenManager: tokenManager,
		cfg:          cfg,
		monitor:      mon,
		wsClient:     wsClient,
	}
}

// SetEngine injects the trading engine (called after engine is created in main).
func (h *Handler) SetEngine(e *trader.Engine) {
	h.engine = e
}

// GET /api/balance
func (h *Handler) GetBalance(c *gin.Context) {
	bal, err := agent.GetAccountBalance(c.Request.Context(), h.client, h.db)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, bal)
}

// GET /api/orders?sync=true
// sync=true 이면 KIS 체결 내역을 먼저 동기화 (PENDING → FILLED/PARTIALLY_FILLED 갱신)
func (h *Handler) GetOrders(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var syncError string
	if c.Query("sync") == "true" {
		days, _ := strconv.Atoi(c.DefaultQuery("days", "1"))
		if days < 1 || days > 90 {
			days = 1
		}
		endDate := time.Now().Format("20060102")
		startDate := time.Now().AddDate(0, 0, -(days - 1)).Format("20060102")
		syncCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		if _, err := agent.GetOrderHistory(syncCtx, h.client, h.db, startDate, endDate); err != nil {
			syncError = err.Error()
		}
	}

	orders, err := agent.GetLocalOrderHistory(c.Request.Context(), h.db, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if orders == nil {
		orders = []models.Order{}
	}
	resp := gin.H{"orders": orders, "limit": limit, "offset": offset}
	if syncError != "" {
		resp["sync_error"] = syncError
	}
	c.JSON(http.StatusOK, resp)
}

// POST /api/orders/:id/cancel — KIS 미체결 주문 취소 (TTTC0084R 확인 후 TTTC0013U 취소 요청)
// 이미 체결된 주문(FILLED)이나 존재하지 않는 KIS 주문번호는 오류 반환.
func (h *Handler) CancelOrder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	result, err := agent.CancelOrder(c.Request.Context(), h.client, h.db, id)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"order_id":     result.OrderID,
		"kis_order_id": result.KISOrderID,
		"status":       "CANCELLED",
	})
}

// DELETE /api/orders/:id
func (h *Handler) DeleteOrder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	res, err := h.db.ExecContext(c.Request.Context(), `DELETE FROM orders WHERE id = ?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// DELETE /api/logs/kis/:id
func (h *Handler) DeleteKISLog(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	res, err := h.db.ExecContext(c.Request.Context(), `DELETE FROM kis_api_logs WHERE id = ?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "log not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// POST /api/orders — place a buy/sell order
// Optional: target_pct and stop_pct register a position for real-time monitoring.
func (h *Handler) PlaceOrder(c *gin.Context) {
	var req struct {
		StockCode string  `json:"stock_code" binding:"required"`
		OrderType string  `json:"order_type" binding:"required"`
		Qty       int     `json:"qty" binding:"required,min=1"`
		Price     float64 `json:"price"`
		TargetPct float64 `json:"target_pct"` // 목표 수익률 (%)
		StopPct   float64 `json:"stop_pct"`   // 손절 비율 (%)
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.db.GetSetting(c.Request.Context(), "trading_enabled") == "false" {
		c.JSON(http.StatusForbidden, gin.H{"error": "거래가 비활성화되어 있습니다. 설정에서 Trading을 ON으로 변경하세요."})
		return
	}

	var orderType models.OrderType
	switch req.OrderType {
	case "BUY":
		orderType = models.OrderTypeBuy
	case "SELL":
		orderType = models.OrderTypeSell
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_type must be BUY or SELL"})
		return
	}

	divn := "00" // 지정가
	if req.Price == 0 {
		divn = "01" // 시장가
	}

	result, err := agent.PlaceOrder(c.Request.Context(), h.client, h.db, agent.PlaceOrderRequest{
		StockCode: req.StockCode,
		OrderType: orderType,
		Qty:       req.Qty,
		Price:     req.Price,
		OrderDivn: divn,
		TargetPct: req.TargetPct,
		StopPct:   req.StopPct,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// BUY 주문이고 target/stop 설정 시 → 모니터링 등록 (체결가는 주문가로 근사)
	// 실제 체결가는 WebSocket 체결통보로 업데이트됨.
	if orderType == models.OrderTypeBuy && req.TargetPct > 0 && req.StopPct > 0 && h.monitor != nil {
		filledPrice := req.Price
		if filledPrice <= 0 {
			// 시장가 주문: 현재가로 목표/손절 계산 (이후 체결통보로 재보정 가능)
			if info, priceErr := h.client.GetStockPrice(c.Request.Context(), req.StockCode); priceErr == nil {
				if p := parseFloat(info.CurrentPrice); p > 0 {
					filledPrice = p
				}
			}
		}
		if filledPrice > 0 {
			entry := monitor.MonitoredEntry{
				StockCode:   req.StockCode,
				StockName:   req.StockCode, // 종목명은 DB 동기화 전까지 코드로 대체
				FilledPrice: filledPrice,
				TargetPrice: filledPrice * (1 + req.TargetPct/100),
				StopPrice:   filledPrice * (1 - req.StopPct/100),
				OrderID:     result.OrderID,
			}
			if regErr := h.monitor.Register(c.Request.Context(), entry); regErr != nil {
				// 등록 실패는 치명적이지 않음 — 로그만 남김
				_ = regErr
			}
		}
	}

	c.JSON(http.StatusCreated, result)
}

// GET /api/server/status — 통합 서버 상태 (시장개장/WebSocket연결/모니터링 수)
func (h *Handler) GetServerStatus(c *gin.Context) {
	now := time.Now().In(agent.KSTLocation())

	marketOpen := false
	if wd := now.Weekday(); wd != time.Saturday && wd != time.Sunday {
		min := now.Hour()*60 + now.Minute()
		if min >= 9*60 && min < 15*60+30 {
			marketOpen = true
		}
	}

	wsConnected := false
	if h.wsClient != nil {
		wsConnected = h.wsClient.IsConnected()
	}

	monitoredCount := 0
	if h.monitor != nil {
		monitoredCount = h.monitor.Count()
	}

	// Available cash from balance
	availableCash := float64(0)
	if bal, err := h.client.GetInquireBalance(c.Request.Context()); err == nil {
		availableCash = parseFloat(bal.DepositAmt)
	}

	tradingEnabled := h.db.GetSetting(c.Request.Context(), "trading_enabled") != "false"

	traderState := string(trader.StateIdle)
	if h.engine != nil {
		traderState = string(h.engine.GetState())
	}

	c.JSON(http.StatusOK, gin.H{
		"market_open":     marketOpen,
		"trading_enabled": tradingEnabled,
		"available_cash":  availableCash,
		"ws_connected":    wsConnected,
		"monitored_count": monitoredCount,
		"trader_state":    traderState,
	})
}

// GET /api/monitor/positions — 현재 모니터링 중인 포지션 목록
func (h *Handler) GetMonitorPositions(c *gin.Context) {
	if h.monitor == nil {
		c.JSON(http.StatusOK, gin.H{"positions": []any{}})
		return
	}
	positions := h.monitor.List()
	if positions == nil {
		positions = []models.MonitoredPosition{}
	}
	c.JSON(http.StatusOK, gin.H{"positions": positions})
}

// DELETE /api/monitor/positions/:code — 모니터링 포지션 제거
func (h *Handler) RemoveMonitorPosition(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stock code required"})
		return
	}
	if h.monitor != nil {
		h.monitor.Remove(c.Request.Context(), code)
	}
	c.JSON(http.StatusOK, gin.H{"removed": code})
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// GET /api/positions — KIS 실시간 보유 종목 조회 (inquire-balance output1)
func (h *Handler) GetPositions(c *gin.Context) {
	holdings, err := h.client.GetHoldings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"positions": holdings})
}

// GET /api/logs/kis?limit=N&summary=true
// summary=true 이면 raw_response 필드를 제외한 요약 형태로 반환
// 호출 시 2일 이상 된 로그는 자동 삭제됨
func (h *Handler) GetKISLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	summary := c.Query("summary") == "true"

	h.db.ExecContext(c.Request.Context(),
		`DELETE FROM kis_api_logs WHERE timestamp < datetime('now', '-2 days')`)

	rows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT id, endpoint, error_code, error_message, raw_response, timestamp
		 FROM kis_api_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var logs []models.KISAPILog
	for rows.Next() {
		var l models.KISAPILog
		if err := rows.Scan(&l.ID, &l.Endpoint, &l.ErrorCode, &l.ErrorMsg, &l.RawResponse, &l.Timestamp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if summary {
			l.RawResponse = "" // 요약 모드에서는 raw 응답 제외
		}
		logs = append(logs, l)
	}
	if logs == nil {
		logs = []models.KISAPILog{}
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// GET /api/stock/:code — 현재가 + MA5 + MA20
func (h *Handler) GetStock(c *gin.Context) {
	code := c.Param("code")
	info, err := agent.GetStockInfo(c.Request.Context(), h.client, code)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

// GET /api/stock/:code/chart?interval=1m|5m|1h
func (h *Handler) GetStockChart(c *gin.Context) {
	code := c.Param("code")
	interval := c.DefaultQuery("interval", "1m")
	candles, err := agent.GetChart(c.Request.Context(), h.client, code, interval)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stock_code": code, "interval": interval, "candles": candles})
}

// GET /api/settings — 서버 상태 및 런타임 설정 조회
func (h *Handler) GetSettings(c *gin.Context) {
	accountNo := h.cfg.KISAccountNo
	maskedAccount := ""
	if len(accountNo) >= 4 {
		maskedAccount = "****" + accountNo[len(accountNo)-4:]
	}

	wsConnected := false
	if h.wsClient != nil {
		wsConnected = h.wsClient.IsConnected()
	}

	tradingEnabled := h.db.GetSetting(c.Request.Context(), "trading_enabled") != "false"
	rankingExclCls := h.db.GetSetting(c.Request.Context(), "ranking_excl_cls")
	if rankingExclCls == "" {
		rankingExclCls = "1111111111"
	}

	ts, _ := h.db.GetTradingSettings(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"account_no":           maskedAccount,
		"account_type":         h.cfg.KISAccountType,
		"kis_configured":       h.cfg.KISAppKey != "" && h.cfg.KISAppSecret != "",
		"hts_id_configured":    h.cfg.KISHTSID != "",
		"anthropic_configured": h.cfg.AnthropicAPIKey != "",
		"mqtt_broker_url":      h.cfg.MQTTBrokerURL,
		"mqtt_client_id":       h.cfg.MQTTClientID,
		"ws_connected":         wsConnected,
		"trading_enabled":      tradingEnabled,
		"ranking_excl_cls":     rankingExclCls,
		// Autonomous trading settings
		"take_profit_pct":              ts.TakeProfitPct,
		"stop_loss_pct":                ts.StopLossPct,
		"ranking_types":                ts.RankingTypes,
		"ranking_price_min":            ts.RankingPriceMin,
		"ranking_price_max":            ts.RankingPriceMax,
		"max_positions":                ts.MaxPositions,
		"order_amount_pct":             ts.OrderAmountPct,
		"sell_conditions":              ts.SellConditions,
		"indicator_check_interval_min": ts.IndicatorCheckIntervalMin,
		"indicator_rsi_sell_threshold": ts.IndicatorRSISellThreshold,
		"indicator_macd_bearish_sell":  ts.IndicatorMACDBearishSell,
		"claude_model":                 ts.ClaudeModel,
	})
}

// PATCH /api/settings — 런타임 설정 업데이트
func (h *Handler) UpdateSettings(c *gin.Context) {
	var req struct {
		TradingEnabled *bool  `json:"trading_enabled"`
		RankingExclCls string `json:"ranking_excl_cls"`
		// Autonomous trading settings (all optional)
		TakeProfitPct             *float64 `json:"take_profit_pct"`
		StopLossPct               *float64 `json:"stop_loss_pct"`
		RankingTypes              []string `json:"ranking_types"`
		RankingPriceMin           string   `json:"ranking_price_min"`
		RankingPriceMax           string   `json:"ranking_price_max"`
		MaxPositions              *int     `json:"max_positions"`
		OrderAmountPct            *float64 `json:"order_amount_pct"`
		SellConditions            []string `json:"sell_conditions"`
		IndicatorCheckIntervalMin *int     `json:"indicator_check_interval_min"`
		IndicatorRSISellThreshold *float64 `json:"indicator_rsi_sell_threshold"`
		IndicatorMACDBearishSell  *bool    `json:"indicator_macd_bearish_sell"`
		ClaudeModel               string   `json:"claude_model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	changed := false

	save := func(key, val string) bool {
		if err := h.db.SetSetting(ctx, key, val); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "저장 실패: " + err.Error()})
			return false
		}
		changed = true
		return true
	}

	if req.TradingEnabled != nil {
		val := "true"
		if !*req.TradingEnabled {
			val = "false"
		}
		if !save("trading_enabled", val) {
			return
		}
	}

	if req.RankingExclCls != "" {
		if len(req.RankingExclCls) != 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ranking_excl_cls는 10자리 문자열이어야 합니다"})
			return
		}
		if !save("ranking_excl_cls", req.RankingExclCls) {
			return
		}
	}

	if req.TakeProfitPct != nil {
		if !save("take_profit_pct", strconv.FormatFloat(*req.TakeProfitPct, 'f', -1, 64)) {
			return
		}
	}
	if req.StopLossPct != nil {
		if !save("stop_loss_pct", strconv.FormatFloat(*req.StopLossPct, 'f', -1, 64)) {
			return
		}
	}
	if len(req.RankingTypes) > 0 {
		b, _ := json.Marshal(req.RankingTypes)
		if !save("ranking_types", string(b)) {
			return
		}
	}
	if req.RankingPriceMin != "" {
		if !save("ranking_price_min", req.RankingPriceMin) {
			return
		}
	}
	if req.RankingPriceMax != "" {
		if !save("ranking_price_max", req.RankingPriceMax) {
			return
		}
	}
	if req.MaxPositions != nil {
		if !save("max_positions", strconv.Itoa(*req.MaxPositions)) {
			return
		}
	}
	if req.OrderAmountPct != nil {
		if !save("order_amount_pct", strconv.FormatFloat(*req.OrderAmountPct, 'f', -1, 64)) {
			return
		}
	}
	if len(req.SellConditions) > 0 {
		b, _ := json.Marshal(req.SellConditions)
		if !save("sell_conditions", string(b)) {
			return
		}
	}
	if req.IndicatorCheckIntervalMin != nil {
		if !save("indicator_check_interval_min", strconv.Itoa(*req.IndicatorCheckIntervalMin)) {
			return
		}
	}
	if req.IndicatorRSISellThreshold != nil {
		if !save("indicator_rsi_sell_threshold", strconv.FormatFloat(*req.IndicatorRSISellThreshold, 'f', -1, 64)) {
			return
		}
	}
	if req.IndicatorMACDBearishSell != nil {
		val := "false"
		if *req.IndicatorMACDBearishSell {
			val = "true"
		}
		if !save("indicator_macd_bearish_sell", val) {
			return
		}
	}
	if req.ClaudeModel != "" {
		if !save("claude_model", req.ClaudeModel) {
			return
		}
	}

	if !changed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "변경할 항목이 없습니다"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "설정이 저장되었습니다."})
}

// GET /api/orders/feasibility?code=:code — 주문가능수량 및 주문가능금액 조회 (TTTC8908R)
// qty > 0 이면 주문 가능. qty == 0 이면 available_cash 기준으로 종목 재선정.
func (h *Handler) GetFeasibility(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code query param is required"})
		return
	}
	result, err := agent.CheckOrderFeasibility(c.Request.Context(), h.client, code)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"orderable_qty":  result.OrderableQty,
		"available_cash": result.AvailableCash,
	})
}

// resolvePriceFilter returns priceMin/priceMax for ranking API calls.
// use_balance_filter=true: 잔액 API(TTTC8434R)로 예수금 조회 후 priceMax로 자동 설정.
// price_min/price_max: 직접 입력값 (use_balance_filter가 true이면 무시됨).
// 잔액 조회 실패 또는 예수금=0이면 필터 미적용(빈값 반환).
func (h *Handler) resolvePriceFilter(c *gin.Context) (priceMin, priceMax string) {
	if c.Query("use_balance_filter") == "true" {
		summary, err := h.client.GetInquireBalance(c.Request.Context())
		if err == nil && summary.DepositAmt != "" && summary.DepositAmt != "0" {
			return "", summary.DepositAmt
		}
		return "", ""
	}
	return c.Query("price_min"), c.Query("price_max")
}

// GET /api/ranking/volume?market=J&sort=0 — 거래량 순위 (FHPST01710000, max 30)
// sort: 0=평균거래량(default), 1=거래량증가율, 2=거래회전율, 3=거래대금순
// price_min/price_max: 가격 범위 직접 입력 (원). use_balance_filter=true: 예수금 기준 자동 설정.
// ETF/ETN/우선주 등 비정상 종목은 항상 제외됨.
func (h *Handler) GetVolumeRank(c *gin.Context) {
	market := c.DefaultQuery("market", "J")
	sort := c.DefaultQuery("sort", "0")
	priceMin, priceMax := h.resolvePriceFilter(c)
	excludeCls := h.db.GetSetting(c.Request.Context(), "ranking_excl_cls")
	items, err := agent.GetVolumeRank(c.Request.Context(), h.client, market, sort, priceMin, priceMax, excludeCls)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranking": items})
}

// GET /api/ranking/strength?market=0000 — 체결강도 상위 (FHPST01680000, max 30)
// market: 0000=전체(default), 0001=거래소, 1001=코스닥, 2001=코스피200
// price_min/price_max: 가격 범위 직접 입력 (원). use_balance_filter=true: 예수금 기준 자동 설정.
// ETF/ETN/우선주 등 비정상 종목은 항상 제외 시도.
func (h *Handler) GetStrengthRank(c *gin.Context) {
	market := c.DefaultQuery("market", "0000")
	priceMin, priceMax := h.resolvePriceFilter(c)
	excludeCls := h.db.GetSetting(c.Request.Context(), "ranking_excl_cls")
	items, err := agent.GetStrengthRank(c.Request.Context(), h.client, market, priceMin, priceMax, excludeCls)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranking": items})
}

// GET /api/ranking/exec-count?market=0000&sort=0 — 대량체결건수 상위 (FHKST190900C0, max 30)
// sort: 0=매수상위(default), 1=매도상위
// price_min/price_max: 가격 범위 직접 입력 (원). use_balance_filter=true: 예수금 기준 자동 설정.
// ETF/ETN/우선주 등 비정상 종목은 항상 제외 시도.
func (h *Handler) GetExecCountRank(c *gin.Context) {
	market := c.DefaultQuery("market", "0000")
	sort := c.DefaultQuery("sort", "0")
	priceMin, priceMax := h.resolvePriceFilter(c)
	excludeCls := h.db.GetSetting(c.Request.Context(), "ranking_excl_cls")
	items, err := agent.GetExecCountRank(c.Request.Context(), h.client, market, sort, priceMin, priceMax, excludeCls)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranking": items})
}

// GET /api/ranking/disparity?market=0000&period=20&sort=0 — 이격도 순위 (FHPST01780000, max 30)
// period: 5, 10, 20(default), 60, 120 / sort: 0=이격도상위(default), 1=이격도하위
// price_min/price_max: 가격 범위 직접 입력 (원). use_balance_filter=true: 예수금 기준 자동 설정.
// ETF/ETN/우선주 등 비정상 종목은 항상 제외 시도.
func (h *Handler) GetDisparityRank(c *gin.Context) {
	market := c.DefaultQuery("market", "0000")
	period := c.DefaultQuery("period", "20")
	sort := c.DefaultQuery("sort", "0")
	priceMin, priceMax := h.resolvePriceFilter(c)
	excludeCls := h.db.GetSetting(c.Request.Context(), "ranking_excl_cls")
	items, err := agent.GetDisparityRank(c.Request.Context(), h.client, market, period, sort, priceMin, priceMax, excludeCls)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranking": items})
}

// GET /api/market/status — 현재 장운영 여부 조회
// Response: { "is_open": bool, "checked_at": RFC3339, "reason": "open"|"weekend"|"outside_hours"|"holiday"|"check_failed" }
func (h *Handler) GetMarketStatus(c *gin.Context) {
	now := time.Now().In(agent.KSTLocation())
	checkedAt := now.Format(time.RFC3339)

	if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
		c.JSON(http.StatusOK, gin.H{"is_open": false, "checked_at": checkedAt, "reason": "weekend"})
		return
	}

	openMinute := now.Hour()*60 + now.Minute()
	if openMinute < 9*60 || openMinute >= 15*60+30 {
		c.JSON(http.StatusOK, gin.H{"is_open": false, "checked_at": checkedAt, "reason": "outside_hours"})
		return
	}

	isOpen, err := agent.IsMarketOpen(c.Request.Context(), h.client)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"is_open": false, "checked_at": checkedAt, "reason": "check_failed"})
		return
	}
	if !isOpen {
		c.JSON(http.StatusOK, gin.H{"is_open": false, "checked_at": checkedAt, "reason": "holiday"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"is_open": true, "checked_at": checkedAt, "reason": "open"})
}

// GET /api/debug/balance — KIS 잔고 API 원본 응답 확인용 (필드명 디버깅)
func (h *Handler) DebugRawBalance(c *gin.Context) {
	raw, err := h.client.GetRawBalance(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

// POST /api/debug/ws — WebSocket 수동 연결 (approval key 발급 후 StartWithReconnect)
func (h *Handler) DebugWSConnect(c *gin.Context) {
	if h.wsClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "WebSocket 클라이언트가 초기화되지 않았습니다"})
		return
	}
	if h.wsClient.IsConnected() {
		c.JSON(http.StatusOK, gin.H{"message": "이미 연결되어 있습니다"})
		return
	}
	key, err := h.client.GetApprovalKey(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "approval key 발급 실패: " + err.Error()})
		return
	}
	h.wsClient.SetApprovalKey(key)
	go h.wsClient.StartWithReconnect(context.Background())
	c.JSON(http.StatusOK, gin.H{"message": "WebSocket 연결 시작"})
}

// DELETE /api/debug/ws — WebSocket 수동 해제
func (h *Handler) DebugWSDisconnect(c *gin.Context) {
	if h.wsClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "WebSocket 클라이언트가 초기화되지 않았습니다"})
		return
	}
	h.wsClient.Disconnect()
	c.JSON(http.StatusOK, gin.H{"message": "WebSocket 해제"})
}

// POST /api/debug/price — 가짜 가격 이벤트 주입 → Monitor.HandlePrice() (is_test: true)
// Body: {"stock_code": "005930", "price": 73500}
func (h *Handler) DebugInjectPrice(c *gin.Context) {
	var req struct {
		StockCode string  `json:"stock_code" binding:"required"`
		Price     float64 `json:"price" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "모니터가 초기화되지 않았습니다"})
		return
	}
	h.monitor.HandlePrice(req.StockCode, req.Price, true)
	c.JSON(http.StatusOK, gin.H{"stock_code": req.StockCode, "price": req.Price})
}

// POST /api/debug/monitor — KIS 주문 없이 모니터 포지션 직접 등록
// Body: {"stock_code":"005930","stock_name":"삼성전자","filled_price":70000,"target_pct":3.0,"stop_pct":2.0}
func (h *Handler) DebugRegisterMonitor(c *gin.Context) {
	var req struct {
		StockCode   string  `json:"stock_code" binding:"required"`
		StockName   string  `json:"stock_name" binding:"required"`
		FilledPrice float64 `json:"filled_price" binding:"required"`
		TargetPct   float64 `json:"target_pct" binding:"required"`
		StopPct     float64 `json:"stop_pct" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "모니터가 초기화되지 않았습니다"})
		return
	}
	entry := monitor.MonitoredEntry{
		StockCode:   req.StockCode,
		StockName:   req.StockName,
		FilledPrice: req.FilledPrice,
		TargetPrice: req.FilledPrice * (1 + req.TargetPct/100),
		StopPrice:   req.FilledPrice * (1 - req.StopPct/100),
		OrderID:     0,
	}
	if err := h.monitor.Register(c.Request.Context(), entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"stock_code":   entry.StockCode,
		"stock_name":   entry.StockName,
		"filled_price": entry.FilledPrice,
		"target_price": entry.TargetPrice,
		"stop_price":   entry.StopPrice,
	})
}

// POST /api/debug/liquidate — LiquidateAll 수동 트리거 (실제 KIS 매도 API 호출됨)
func (h *Handler) DebugLiquidate(c *gin.Context) {
	if h.monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "모니터가 초기화되지 않았습니다"})
		return
	}
	go h.monitor.LiquidateAll(context.Background())
	c.JSON(http.StatusOK, gin.H{"message": "청산 시작 (비동기 실행)"})
}

// GET /api/reports — 일일 리포트 목록 (날짜 내림차순)
func (h *Handler) GetReports(c *gin.Context) {
	rows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT id, report_date, created_at FROM reports ORDER BY report_date DESC LIMIT 30`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var reports []models.Report
	for rows.Next() {
		var r models.Report
		if err := rows.Scan(&r.ID, &r.ReportDate, &r.CreatedAt); err == nil {
			reports = append(reports, r)
		}
	}
	if reports == nil {
		reports = []models.Report{}
	}
	c.JSON(http.StatusOK, gin.H{"reports": reports})
}

// GET /api/reports/:date — 특정 날짜 리포트 전문 조회 (YYYY-MM-DD)
func (h *Handler) GetReport(c *gin.Context) {
	date := c.Param("date")
	if date == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date parameter required"})
		return
	}

	var r models.Report
	err := h.db.QueryRowContext(c.Request.Context(),
		`SELECT id, report_date, content, created_at FROM reports WHERE report_date = ?`, date,
	).Scan(&r.ID, &r.ReportDate, &r.Content, &r.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		return
	}
	c.JSON(http.StatusOK, r)
}
