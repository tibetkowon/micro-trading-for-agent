package kis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/logger"
)

// Client is the KIS API HTTP client.
// All responses are rate-limited; errors are persisted to kis_api_logs.
type Client struct {
	mu           sync.RWMutex
	baseURL      string
	isMock       bool
	mockBaseURL  string
	realBaseURL  string
	appKey       string
	appSecret    string
	accountNo    string
	accountType  string
	tokenManager *TokenManager
	rateLimiter  *RateLimiter
	db           *database.DB
	httpClient   *http.Client
}

// NewClient creates a fully configured KIS API client.
func NewClient(
	realBaseURL, mockBaseURL string,
	isMock bool,
	appKey, appSecret, accountNo, accountType string,
	tokenManager *TokenManager,
	db *database.DB,
) *Client {
	baseURL := realBaseURL
	if isMock {
		baseURL = mockBaseURL
	}
	return &Client{
		baseURL:      baseURL,
		isMock:       isMock,
		mockBaseURL:  mockBaseURL,
		realBaseURL:  realBaseURL,
		appKey:       appKey,
		appSecret:    appSecret,
		accountNo:    accountNo,
		accountType:  accountType,
		tokenManager: tokenManager,
		// KIS allows up to 20 TPS; use 15 to stay safely under the limit.
		rateLimiter: NewRateLimiter(15, 15),
		db:          db,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// IsMock returns the current trading mode.
func (c *Client) IsMock() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isMock
}

// SetMock switches between mock (모의투자) and real (실전투자) mode.
// It updates the base URL on both the client and the token manager,
// then re-issues a token for the new environment.
func (c *Client) SetMock(ctx context.Context, isMock bool) error {
	c.mu.Lock()
	c.isMock = isMock
	if isMock {
		c.baseURL = c.mockBaseURL
	} else {
		c.baseURL = c.realBaseURL
	}
	c.mu.Unlock()

	c.tokenManager.SetBaseURL(c.baseURL)
	if _, err := c.tokenManager.IssueToken(ctx); err != nil {
		return fmt.Errorf("re-issue token after mode switch: %w", err)
	}
	logger.Info("KIS mode switched", map[string]any{"is_mock": isMock})
	return nil
}

// trID returns the appropriate TR ID for the current mode.
// 모의투자 TR IDs use a "V" prefix instead of "T".
func (c *Client) trID(real, mock string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.isMock {
		return mock
	}
	return real
}

// --- Request/Response DTOs ---

// StockPriceResponse holds the current price data for a stock.
type StockPriceResponse struct {
	StockCode    string `json:"stck_shrn_iscd"` // 단축종목코드
	CurrentPrice string `json:"stck_prpr"`      // 주식 현재가
	ChangeRate   string `json:"prdy_ctrt"`      // 전일대비율
	Volume       string `json:"acml_vol"`       // 누적 거래량
}

// BalanceResponse holds account balance details.
type BalanceResponse struct {
	TotalEval       string `json:"tot_evlu_amt"`  // 총평가금액
	AvailableAmount string `json:"ord_psbl_cash"` // 주문가능현금
	ProfitRate      string `json:"evlu_pfls_rt"`  // 평가손익률
}

// OrderRequest is the payload for placing a buy/sell order.
type OrderRequest struct {
	StockCode string `json:"pdno"`     // 종목코드
	OrderDivn string `json:"ord_dvsn"` // 주문구분 (00=지정가, 01=시장가)
	Qty       string `json:"ord_qty"`  // 주문수량
	Price     string `json:"ord_unpr"` // 주문단가 (시장가일 때 "0")
}

// OrderResponse is returned by the KIS order API.
type OrderResponse struct {
	KISOrderID string `json:"odno"`    // KIS 주문번호
	OrderTime  string `json:"ord_tmd"` // 주문시각
}

// --- Public API methods ---

// GetStockPrice fetches the current price for the given stock code.
func (c *Client) GetStockPrice(ctx context.Context, stockCode string) (*StockPriceResponse, error) {
	endpoint := "/uapi/domestic-stock/v1/quotations/inquire-price"
	params := fmt.Sprintf("?FID_COND_MRKT_DIV_CODE=J&FID_INPUT_ISCD=%s", stockCode)

	// 주식 현재가 TR ID는 모의/실전 동일
	raw, err := c.get(ctx, endpoint, params, "FHKST01010100")
	if err != nil {
		return nil, err
	}

	var result struct {
		Output  StockPriceResponse `json:"output"`
		MsgCode string             `json:"msg_cd"`
		Msg     string             `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse stock price: %w", err)
	}
	return &result.Output, nil
}

// GetBalance fetches the current account balance.
func (c *Client) GetBalance(ctx context.Context) (*BalanceResponse, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-psbl-order"
	params := fmt.Sprintf("?CANO=%s&ACNT_PRDT_CD=%s&PDNO=&ORD_UNPR=&ORD_QTY=&OVRS_ICLD_YN=N&CMA_EVLU_AMT_ICLD_YN=N&ITEM_OVRS_EXCG_CD=",
		c.accountNo, c.accountType)

	raw, err := c.get(ctx, endpoint, params, c.trID("TTTC8908R", "VTTC8908R"))
	if err != nil {
		return nil, err
	}

	var result struct {
		Output  BalanceResponse `json:"output"`
		MsgCode string          `json:"msg_cd"`
		Msg     string          `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse balance: %w", err)
	}
	return &result.Output, nil
}

// PlaceBuyOrder places a buy order.
func (c *Client) PlaceBuyOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	return c.placeOrder(ctx, req,
		c.trID("TTTC0802U", "VTTC0802U"),
		"/uapi/domestic-stock/v1/trading/order-cash")
}

// PlaceSellOrder places a sell order.
func (c *Client) PlaceSellOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	return c.placeOrder(ctx, req,
		c.trID("TTTC0801U", "VTTC0801U"),
		"/uapi/domestic-stock/v1/trading/order-cash")
}

// GetOrderHistory fetches recent order history.
func (c *Client) GetOrderHistory(ctx context.Context) ([]map[string]any, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-daily-ccld"
	params := fmt.Sprintf("?CANO=%s&ACNT_PRDT_CD=%s&INQR_STRT_DT=&INQR_END_DT=&SLL_BUY_DVSN_CD=00&INQR_DVSN=00&PDNO=&CCLD_DVSN=01&ORD_GNO_BRNO=&ODNO=&CANC_YN=N&CTX_AREA_FK100=&CTX_AREA_NK100=",
		c.accountNo, c.accountType)

	raw, err := c.get(ctx, endpoint, params, c.trID("TTTC8001R", "VTTC8001R"))
	if err != nil {
		return nil, err
	}

	var result struct {
		Output  []map[string]any `json:"output1"`
		MsgCode string           `json:"msg_cd"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse order history: %w", err)
	}
	return result.Output, nil
}

// --- Internal helpers ---

func (c *Client) get(ctx context.Context, endpoint, queryParams, trID string) ([]byte, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	tok, err := c.tokenManager.GetCurrentToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	c.mu.RLock()
	baseURL := c.baseURL
	c.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+endpoint+queryParams, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, tok.AccessToken, trID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.logAPIError(endpoint, fmt.Sprintf("HTTP-%d", resp.StatusCode), string(raw))
		return nil, fmt.Errorf("KIS GET %s returned %d", endpoint, resp.StatusCode)
	}
	return raw, nil
}

func (c *Client) placeOrder(ctx context.Context, req OrderRequest, trID, endpoint string) (*OrderResponse, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	tok, err := c.tokenManager.GetCurrentToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	body, _ := json.Marshal(map[string]string{
		"CANO":         c.accountNo,
		"ACNT_PRDT_CD": c.accountType,
		"PDNO":         req.StockCode,
		"ORD_DVSN":     req.OrderDivn,
		"ORD_QTY":      req.Qty,
		"ORD_UNPR":     req.Price,
	})

	c.mu.RLock()
	baseURL := c.baseURL
	c.mu.RUnlock()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq, tok.AccessToken, trID)
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.logAPIError(endpoint, fmt.Sprintf("HTTP-%d", resp.StatusCode), string(raw))
		return nil, fmt.Errorf("KIS POST %s returned %d", endpoint, resp.StatusCode)
	}

	var result struct {
		Output  OrderResponse `json:"output"`
		MsgCode string        `json:"msg_cd"`
		Msg     string        `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse order response: %w", err)
	}

	if result.MsgCode != "" && result.MsgCode != "MABC000" {
		c.logAPIError(endpoint, result.MsgCode, string(raw))
		return nil, fmt.Errorf("KIS order error [%s]: %s", result.MsgCode, result.Msg)
	}

	return &result.Output, nil
}

func (c *Client) setHeaders(req *http.Request, accessToken, trID string) {
	req.Header.Set("authorization", "Bearer "+accessToken)
	req.Header.Set("appkey", c.appKey)
	req.Header.Set("appsecret", c.appSecret)
	req.Header.Set("tr_id", trID)
	req.Header.Set("custtype", "P")
}

// logAPIError persists a KIS API error to the database and structured logger.
// Per CLAUDE.md: Error Code + Timestamp + raw KIS API Response Message are REQUIRED.
func (c *Client) logAPIError(endpoint, errorCode, rawResponse string) {
	logger.KISError(endpoint, errorCode, rawResponse)
	_, err := c.db.Exec(
		`INSERT INTO kis_api_logs (endpoint, error_code, error_message, raw_response, timestamp)
		 VALUES (?, ?, ?, ?, ?)`,
		endpoint, errorCode, extractMsg(rawResponse), rawResponse, time.Now().UTC(),
	)
	if err != nil {
		logger.Error("failed to persist KIS API error log", map[string]any{"error": err.Error()})
	}
}

// extractMsg attempts to pull msg1 from raw JSON for a human-readable message.
func extractMsg(raw string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	if v, ok := m["msg1"].(string); ok {
		return v
	}
	return ""
}
