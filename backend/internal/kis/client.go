package kis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/logger"
)

// Client is the KIS API HTTP client.
// All responses are rate-limited; errors are persisted to kis_api_logs.
type Client struct {
	baseURL      string
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
	baseURL string,
	appKey, appSecret, accountNo, accountType string,
	tokenManager *TokenManager,
	db *database.DB,
) *Client {
	return &Client{
		baseURL:      baseURL,
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

// --- Request/Response DTOs ---

// StockPriceResponse holds the current price data for a stock.
type StockPriceResponse struct {
	StockCode    string `json:"stck_shrn_iscd"` // 단축종목코드
	CurrentPrice string `json:"stck_prpr"`      // 주식 현재가
	ChangeRate   string `json:"prdy_ctrt"`      // 전일대비율
	Volume       string `json:"acml_vol"`       // 누적 거래량
}

// AvailableOrderResponse holds response from inquire-psbl-order (매수가능조회 TTTC8908R).
// Used by the agent right before placing an order for a specific stock.
type AvailableOrderResponse struct {
	OrderableQty  string `json:"ord_psbl_qty"`  // 주문가능수량 (0이면 주문 불가)
	AvailableCash string `json:"ord_psbl_cash"` // 주문가능현금 (재선정 기준 금액)
}

// InquireBalanceOutput2 holds account summary from inquire-balance output2 (TTTC8434R).
type InquireBalanceOutput2 struct {
	TotalEval      string `json:"tot_evlu_amt"`          // 총평가금액
	DepositAmt     string `json:"dnca_tot_amt"`          // 예수금총금액 = 출금가능금액
	AssetChangeAmt string `json:"asst_icdc_amt"`         // 자산증감액
	PrevTotalAsset string `json:"bfdy_tot_asst_evlu_amt"` // 전일총자산평가금액 (수익률 계산용)
}

// HoldingItem holds a single stock position from inquire-balance output1 (보유 종목).
type HoldingItem struct {
	StockCode    string `json:"pdno"`          // 종목코드
	StockName    string `json:"prdt_name"`     // 종목명
	HoldingQty   string `json:"hldg_qty"`      // 보유수량
	AvgPrice     string `json:"pchs_avg_pric"` // 매입평균가
	CurrentPrice string `json:"prpr"`          // 현재가
	EvalAmount   string `json:"evlu_amt"`      // 평가금액
	ProfitLoss   string `json:"evlu_pfls_amt"` // 평가손익
	ProfitRate   string `json:"evlu_erng_rt"`  // 평가수익률
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

// GetAvailableOrder checks order feasibility for a specific stock (매수가능조회 TTTC8908R).
// stockCode: 종목코드 (e.g. "005930"). ORD_DVSN=01(시장가), ORD_UNPR=0.
// Returns ord_psbl_qty (주문가능수량) and ord_psbl_cash (주문가능현금).
func (c *Client) GetAvailableOrder(ctx context.Context, stockCode string) (*AvailableOrderResponse, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-psbl-order"
	params := fmt.Sprintf("?CANO=%s&ACNT_PRDT_CD=%s&PDNO=%s&ORD_UNPR=0&ORD_DVSN=01&CMA_EVLU_AMT_ICLD_YN=N&OVRS_ICLD_YN=N",
		c.accountNo, c.accountType, stockCode)

	raw, err := c.get(ctx, endpoint, params, "TTTC8908R")
	if err != nil {
		return nil, err
	}

	var result struct {
		Output  AvailableOrderResponse `json:"output"`
		MsgCode string                 `json:"msg_cd"`
		Msg     string                 `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse available order: %w", err)
	}
	return &result.Output, nil
}

// GetInquireBalance fetches account balance summary (주식잔고조회).
// output2 contains tot_evlu_amt (총평가금액) and profit/loss summary.
func (c *Client) GetInquireBalance(ctx context.Context) (*InquireBalanceOutput2, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-balance"
	params := fmt.Sprintf("?CANO=%s&ACNT_PRDT_CD=%s&AFHR_FLPR_YN=N&OFL_YN=&INQR_DVSN=01&UNPR_DVSN=01&FUND_STTL_ICLD_YN=N&FNCG_AMT_AUTO_RDPT_YN=N&PRCS_DVSN=00&CTX_AREA_FK100=&CTX_AREA_NK100=",
		c.accountNo, c.accountType)

	raw, err := c.get(ctx, endpoint, params, "TTTC8434R")
	if err != nil {
		return nil, err
	}

	// output2 is an array in KIS API response — take the first element.
	var result struct {
		Output2 []InquireBalanceOutput2 `json:"output2"`
		MsgCode string                  `json:"msg_cd"`
		Msg     string                  `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse inquire balance: %w", err)
	}
	if len(result.Output2) == 0 {
		return &InquireBalanceOutput2{}, nil
	}
	return &result.Output2[0], nil
}

// PlaceBuyOrder places a buy order.
// TR-ID TTTC0012U: 국내주식주문 매수 (신규 TR, 구 TTTC0802U 대체)
func (c *Client) PlaceBuyOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	return c.placeOrder(ctx, req, "TTTC0012U", "/uapi/domestic-stock/v1/trading/order-cash")
}

// PlaceSellOrder places a sell order.
// TR-ID TTTC0011U: 국내주식주문 매도 (신규 TR, 구 TTTC0801U 대체)
func (c *Client) PlaceSellOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	return c.placeOrder(ctx, req, "TTTC0011U", "/uapi/domestic-stock/v1/trading/order-cash")
}

// GetHoldings fetches currently held stock positions from inquire-balance output1 (보유 종목 조회).
func (c *Client) GetHoldings(ctx context.Context) ([]HoldingItem, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-balance"
	params := fmt.Sprintf("?CANO=%s&ACNT_PRDT_CD=%s&AFHR_FLPR_YN=N&OFL_YN=&INQR_DVSN=01&UNPR_DVSN=01&FUND_STTL_ICLD_YN=N&FNCG_AMT_AUTO_RDPT_YN=N&PRCS_DVSN=00&CTX_AREA_FK100=&CTX_AREA_NK100=",
		c.accountNo, c.accountType)

	raw, err := c.get(ctx, endpoint, params, "TTTC8434R")
	if err != nil {
		return nil, err
	}

	var result struct {
		Output1 []HoldingItem `json:"output1"`
		MsgCode string        `json:"msg_cd"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse holdings: %w", err)
	}
	if result.Output1 == nil {
		return []HoldingItem{}, nil
	}
	return result.Output1, nil
}

// GetRawBalance returns the raw JSON response from the inquire-balance endpoint.
// Used for debugging field name mismatches.
func (c *Client) GetRawBalance(ctx context.Context) ([]byte, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-balance"
	params := fmt.Sprintf("?CANO=%s&ACNT_PRDT_CD=%s&AFHR_FLPR_YN=N&OFL_YN=&INQR_DVSN=01&UNPR_DVSN=01&FUND_STTL_ICLD_YN=N&FNCG_AMT_AUTO_RDPT_YN=N&PRCS_DVSN=00&CTX_AREA_FK100=&CTX_AREA_NK100=",
		c.accountNo, c.accountType)
	return c.get(ctx, endpoint, params, "TTTC8434R")
}

// GetOrderHistory fetches recent order history.
func (c *Client) GetOrderHistory(ctx context.Context) ([]map[string]any, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-daily-ccld"
	params := fmt.Sprintf("?CANO=%s&ACNT_PRDT_CD=%s&INQR_STRT_DT=&INQR_END_DT=&SLL_BUY_DVSN_CD=00&INQR_DVSN=00&PDNO=&CCLD_DVSN=01&ORD_GNO_BRNO=&ODNO=&CANC_YN=N&CTX_AREA_FK100=&CTX_AREA_NK100=",
		c.accountNo, c.accountType)

	raw, err := c.get(ctx, endpoint, params, "TTTC8001R")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint+queryParams, nil)
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
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
		RtCd    string        `json:"rt_cd"`  // "0" = 성공
		MsgCode string        `json:"msg_cd"` // 성공: APBK0013, MABC000 등
		Msg     string        `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse order response: %w", err)
	}

	// rt_cd="0" 이 KIS 공식 성공 기준 (msg_cd는 계좌 유형별로 상이: APBK0013, MABC000 등)
	if result.RtCd != "0" {
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
