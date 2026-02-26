package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/micro-trading-for-agent/backend/internal/agent"
	"github.com/micro-trading-for-agent/backend/internal/config"
	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	db           *database.DB
	client       *kis.Client
	tokenManager *kis.TokenManager
	cfg          *config.Config
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(db *database.DB, client *kis.Client, tokenManager *kis.TokenManager, cfg *config.Config) *Handler {
	return &Handler{db: db, client: client, tokenManager: tokenManager, cfg: cfg}
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

	if c.Query("sync") == "true" {
		// 오류가 있어도 DB 조회는 계속 진행
		_, _ = agent.GetOrderHistory(c.Request.Context(), h.client, h.db)
	}

	orders, err := agent.GetLocalOrderHistory(c.Request.Context(), h.db, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if orders == nil {
		orders = []models.Order{}
	}
	c.JSON(http.StatusOK, gin.H{"orders": orders, "limit": limit, "offset": offset})
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

// POST /api/orders — manual order for testing
func (h *Handler) PlaceOrder(c *gin.Context) {
	var req struct {
		StockCode string  `json:"stock_code" binding:"required"`
		OrderType string  `json:"order_type" binding:"required"`
		Qty       int     `json:"qty" binding:"required,min=1"`
		Price     float64 `json:"price"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, result)
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
func (h *Handler) GetKISLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	summary := c.Query("summary") == "true"

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

// GET /api/settings — 읽기 전용 서버 상태 조회 (민감 정보 제외)
func (h *Handler) GetSettings(c *gin.Context) {
	accountNo := h.cfg.KISAccountNo
	maskedAccount := ""
	if len(accountNo) >= 4 {
		maskedAccount = "****" + accountNo[len(accountNo)-4:]
	}

	c.JSON(http.StatusOK, gin.H{
		"account_no":     maskedAccount,
		"kis_configured": h.cfg.KISAppKey != "" && h.cfg.KISAppSecret != "",
		"account_type":   h.cfg.KISAccountType,
	})
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

// GET /api/ranking/volume?market=J&sort=0 — 거래량 순위 (FHPST01710000, max 30)
// sort: 0=평균거래량(default), 1=거래량증가율, 2=거래회전율, 3=거래대금순
func (h *Handler) GetVolumeRank(c *gin.Context) {
	market := c.DefaultQuery("market", "J")
	sort := c.DefaultQuery("sort", "0")
	items, err := agent.GetVolumeRank(c.Request.Context(), h.client, market, sort)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranking": items})
}

// GET /api/ranking/strength?market=0000 — 체결강도 상위 (FHPST01680000, max 30)
// market: 0000=전체(default), 0001=거래소, 1001=코스닥, 2001=코스피200
func (h *Handler) GetStrengthRank(c *gin.Context) {
	market := c.DefaultQuery("market", "0000")
	items, err := agent.GetStrengthRank(c.Request.Context(), h.client, market)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranking": items})
}

// GET /api/ranking/exec-count?market=0000&sort=0 — 대량체결건수 상위 (FHKST190900C0, max 30)
// sort: 0=매수상위(default), 1=매도상위
func (h *Handler) GetExecCountRank(c *gin.Context) {
	market := c.DefaultQuery("market", "0000")
	sort := c.DefaultQuery("sort", "0")
	items, err := agent.GetExecCountRank(c.Request.Context(), h.client, market, sort)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranking": items})
}

// GET /api/ranking/disparity?market=0000&period=20&sort=0 — 이격도 순위 (FHPST01780000, max 30)
// period: 5, 10, 20(default), 60, 120 / sort: 0=이격도상위(default), 1=이격도하위
func (h *Handler) GetDisparityRank(c *gin.Context) {
	market := c.DefaultQuery("market", "0000")
	period := c.DefaultQuery("period", "20")
	sort := c.DefaultQuery("sort", "0")
	items, err := agent.GetDisparityRank(c.Request.Context(), h.client, market, period, sort)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranking": items})
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
