package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/micro-trading-for-agent/backend/internal/agent"
	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	db     *database.DB
	client *kis.Client
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(db *database.DB, client *kis.Client) *Handler {
	return &Handler{db: db, client: client}
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

// GET /api/orders
func (h *Handler) GetOrders(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
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

// GET /api/logs/kis
func (h *Handler) GetKISLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}

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
		logs = append(logs, l)
	}
	if logs == nil {
		logs = []models.KISAPILog{}
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// GET /api/settings — returns settings with sensitive values masked
func (h *Handler) GetSettings(c *gin.Context) {
	rows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT key, value, updated_at FROM settings ORDER BY key`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	sensitiveKeys := map[string]bool{
		"KIS_APP_KEY": true, "KIS_APP_SECRET": true,
	}
	var settings []gin.H
	for rows.Next() {
		var s models.Setting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		val := s.Value
		if sensitiveKeys[s.Key] && len(val) > 4 {
			val = val[:4] + "****"
		}
		settings = append(settings, gin.H{"key": s.Key, "value": val, "updated_at": s.UpdatedAt})
	}
	if settings == nil {
		settings = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"settings": settings})
}

// PUT /api/settings
func (h *Handler) UpdateSettings(c *gin.Context) {
	var req struct {
		Key   string `json:"key" binding:"required"`
		Value string `json:"value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.db.ExecContext(c.Request.Context(),
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		req.Key, req.Value,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "setting updated"})
}
