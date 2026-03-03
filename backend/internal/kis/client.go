package kis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
		// KIS allows up to 20 TPS; burst=1 enforces strict per-request spacing.
		rateLimiter: NewRateLimiter(10, 1),
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
	OrderableQty  string `json:"nrcvb_buy_qty"` // 미수없는매수수량 (TTTC8908R, 미수 미사용 시)
	AvailableCash string `json:"ord_psbl_cash"` // 주문가능현금 (재선정 기준 금액)
}

// InquireBalanceOutput2 holds account summary from inquire-balance output2 (TTTC8434R).
type InquireBalanceOutput2 struct {
	TotalEval      string `json:"tot_evlu_amt"`           // 총평가금액
	DepositAmt     string `json:"dnca_tot_amt"`           // 예수금총금액 = 출금가능금액
	AssetChangeAmt string `json:"asst_icdc_amt"`          // 자산증감액
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

// VolumeRankItem holds one entry from the volume ranking API (FHPST01710000).
type VolumeRankItem struct {
	DataRank     string `json:"data_rank"`      // 순위
	StockCode    string `json:"mksc_shrn_iscd"` // 종목코드
	StockName    string `json:"hts_kor_isnm"`   // 종목명
	CurrentPrice string `json:"stck_prpr"`      // 현재가
	Volume       string `json:"acml_vol"`       // 누적거래량
	AvgVolume    string `json:"avrg_vol"`       // 평균거래량
	VolIncrRate  string `json:"vol_inrt"`       // 거래량증가율
}

// StrengthRankItem holds one entry from the execution strength ranking (FHPST01680000).
type StrengthRankItem struct {
	DataRank     string `json:"data_rank"`      // 순위
	StockCode    string `json:"stck_shrn_iscd"` // 종목코드
	StockName    string `json:"hts_kor_isnm"`   // 종목명
	CurrentPrice string `json:"stck_prpr"`      // 현재가
	Volume       string `json:"acml_vol"`       // 누적거래량
	Strength     string `json:"tday_rltv"`      // 체결강도
	BuyQty       string `json:"shnu_cnqn_smtn"` // 매수체결량합계
	SellQty      string `json:"seln_cnqn_smtn"` // 매도체결량합계
}

// ExecCountRankItem holds one entry from the bulk execution count ranking (FHKST190900C0).
type ExecCountRankItem struct {
	DataRank     string `json:"data_rank"`      // 순위
	StockCode    string `json:"mksc_shrn_iscd"` // 종목코드
	StockName    string `json:"hts_kor_isnm"`   // 종목명
	CurrentPrice string `json:"stck_prpr"`      // 현재가
	Volume       string `json:"acml_vol"`       // 누적거래량
	BuyCount     string `json:"shnu_cntg_csnu"` // 매수체결건수
	SellCount    string `json:"seln_cntg_csnu"` // 매도체결건수
	NetBuyQty    string `json:"ntby_cnqn"`      // 순매수체결량
}

// DisparityRankItem holds one entry from the disparity index ranking (FHPST01780000).
type DisparityRankItem struct {
	DataRank     string `json:"data_rank"`      // 순위
	StockCode    string `json:"mksc_shrn_iscd"` // 종목코드
	StockName    string `json:"hts_kor_isnm"`   // 종목명
	CurrentPrice string `json:"stck_prpr"`      // 현재가
	ChangeRate   string `json:"prdy_ctrt"`      // 전일대비율
	Volume       string `json:"acml_vol"`       // 누적거래량
	D5           string `json:"d5_dsrt"`        // 5일 이격도
	D10          string `json:"d10_dsrt"`       // 10일 이격도
	D20          string `json:"d20_dsrt"`       // 20일 이격도
	D60          string `json:"d60_dsrt"`       // 60일 이격도
	D120         string `json:"d120_dsrt"`      // 120일 이격도
}

// HolidayInfo holds market open/close status for a given date (CTCA0903R).
type HolidayInfo struct {
	BassDate string `json:"bass_dt"` // YYYYMMDD
	IsBizDay string `json:"bzdy_yn"` // Y=영업일, N=휴장
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

// CancellableOrderItem holds one entry from the cancellable-order query (TTTC0084R).
type CancellableOrderItem struct {
	OrdGnoBrno   string `json:"ord_gno_brno"`    // 주문채번지점번호 (KRX_FWDG_ORD_ORGNO로 사용)
	Odno         string `json:"odno"`            // 주문번호
	OrdDvsnCd    string `json:"ord_dvsn_cd"`     // 주문구분코드 (00=지정가, 01=시장가 등)
	OrdUnpr      string `json:"ord_unpr"`        // 주문단가
	OrdQty       string `json:"ord_qty"`         // 주문수량
	PsblQty      string `json:"psbl_qty"`        // 정정/취소 가능 수량
	TotCcldQty   string `json:"tot_ccld_qty"`    // 총 체결 수량
	Pdno         string `json:"pdno"`            // 종목코드
	PrdtName     string `json:"prdt_name"`       // 종목명
	SllBuyDvsnCd string `json:"sll_buy_dvsn_cd"` // 01=매도, 02=매수
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

// GetVolumeRank fetches the volume ranking (거래량 순위 FHPST01710000). Max 30 results.
// market: "J"=KRX(default), "NX"=NXT.
// sort (FID_BLNG_CLS_CODE): "0"=평균거래량(default), "1"=거래량증가율, "2"=평균거래회전율, "3"=거래대금순.
// priceMin/priceMax: 가격 범위 필터 (빈값="" 이면 전체 가격 조회).
// FID_TRGT_EXLS_CLS_CODE=1111111111: 투자위험/경고/주의/관리종목/정리매매/불성실공시/우선주/거래정지/ETF/ETN 모두 제외 → 일반주식만.
func (c *Client) GetVolumeRank(ctx context.Context, market, sort, priceMin, priceMax string) ([]VolumeRankItem, error) {
	endpoint := "/uapi/domestic-stock/v1/quotations/volume-rank"
	params := fmt.Sprintf(
		"?FID_COND_MRKT_DIV_CODE=%s&FID_COND_SCR_DIV_CODE=20171&FID_INPUT_ISCD=0000&FID_DIV_CLS_CODE=0&FID_BLNG_CLS_CODE=%s&FID_TRGT_CLS_CODE=111111111&FID_TRGT_EXLS_CLS_CODE=1111111111&FID_INPUT_PRICE_1=%s&FID_INPUT_PRICE_2=%s&FID_VOL_CNT=&FID_INPUT_DATE_1=",
		market, sort, priceMin, priceMax)

	raw, err := c.get(ctx, endpoint, params, "FHPST01710000")
	if err != nil {
		return nil, err
	}
	var result struct {
		Output  []VolumeRankItem `json:"output"`
		MsgCode string           `json:"msg_cd"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse volume rank: %w", err)
	}
	if result.Output == nil {
		return []VolumeRankItem{}, nil
	}
	return result.Output, nil
}

// GetStrengthRank fetches the execution strength ranking (체결강도 상위 FHPST01680000). Max 30 results.
// market (fid_input_iscd): "0000"=전체(default), "0001"=거래소, "1001"=코스닥, "2001"=코스피200.
// priceMin/priceMax: 가격 범위 필터 (빈값="" 이면 전체 가격 조회).
// fid_trgt_exls_cls_code=1111111111: ETF/ETN/우선주 등 비정상 종목 제외 시도.
func (c *Client) GetStrengthRank(ctx context.Context, market, priceMin, priceMax string) ([]StrengthRankItem, error) {
	endpoint := "/uapi/domestic-stock/v1/ranking/volume-power"
	params := fmt.Sprintf(
		"?fid_cond_mrkt_div_code=J&fid_cond_scr_div_code=20168&fid_input_iscd=%s&fid_div_cls_code=0&fid_input_price_1=%s&fid_input_price_2=%s&fid_vol_cnt=&fid_trgt_cls_code=0&fid_trgt_exls_cls_code=1111111111",
		market, priceMin, priceMax)

	raw, err := c.get(ctx, endpoint, params, "FHPST01680000")
	if err != nil {
		return nil, err
	}
	var result struct {
		Output  []StrengthRankItem `json:"output"`
		MsgCode string             `json:"msg_cd"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse strength rank: %w", err)
	}
	if result.Output == nil {
		return []StrengthRankItem{}, nil
	}
	return result.Output, nil
}

// GetExecCountRank fetches the bulk execution count ranking (대량체결건수 상위 FHKST190900C0). Max 30 results.
// market (fid_input_iscd): "0000"=전체(default), "0001"=거래소, "1001"=코스닥, "2001"=코스피200.
// sort (fid_rank_sort_cls_code): "0"=매수상위(default), "1"=매도상위.
// priceMin/priceMax: 가격 범위 필터 (빈값="" 이면 전체 가격 조회).
// fid_trgt_exls_cls_code=1111111111: ETF/ETN/우선주 등 비정상 종목 제외 시도.
func (c *Client) GetExecCountRank(ctx context.Context, market, sort, priceMin, priceMax string) ([]ExecCountRankItem, error) {
	endpoint := "/uapi/domestic-stock/v1/ranking/bulk-trans-num"
	params := fmt.Sprintf(
		"?fid_cond_mrkt_div_code=J&fid_cond_scr_div_code=11909&fid_input_iscd=%s&fid_div_cls_code=0&fid_rank_sort_cls_code=%s&fid_input_price_1=%s&fid_input_price_2=%s&fid_aply_rang_prc_1=&fid_aply_rang_prc_2=&fid_input_iscd_2=&fid_vol_cnt=&fid_trgt_cls_code=0&fid_trgt_exls_cls_code=1111111111",
		market, sort, priceMin, priceMax)

	raw, err := c.get(ctx, endpoint, params, "FHKST190900C0")
	if err != nil {
		return nil, err
	}
	var result struct {
		Output  []ExecCountRankItem `json:"output"`
		MsgCode string              `json:"msg_cd"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse exec count rank: %w", err)
	}
	if result.Output == nil {
		return []ExecCountRankItem{}, nil
	}
	return result.Output, nil
}

// GetDisparityRank fetches the disparity index ranking (이격도 순위 FHPST01780000). Max 30 results.
// market (fid_input_iscd): "0000"=전체(default), "0001"=거래소, "1001"=코스닥, "2001"=코스피200.
// period (fid_hour_cls_code): "5", "10", "20"(default), "60", "120".
// sort (fid_rank_sort_cls_code): "0"=이격도 상위순(default), "1"=이격도 하위순.
// priceMin/priceMax: 가격 범위 필터 (빈값="" 이면 전체 가격 조회).
// fid_trgt_exls_cls_code=1111111111: ETF/ETN/우선주 등 비정상 종목 제외 시도.
func (c *Client) GetDisparityRank(ctx context.Context, market, period, sort, priceMin, priceMax string) ([]DisparityRankItem, error) {
	endpoint := "/uapi/domestic-stock/v1/ranking/disparity"
	params := fmt.Sprintf(
		"?fid_input_price_2=%s&fid_cond_mrkt_div_code=J&fid_cond_scr_div_code=20178&fid_div_cls_code=6&fid_rank_sort_cls_code=%s&fid_hour_cls_code=%s&fid_input_iscd=%s&fid_trgt_cls_code=0&fid_trgt_exls_cls_code=1111111111&fid_input_price_1=%s&fid_vol_cnt=",
		priceMax, sort, period, market, priceMin)

	raw, err := c.get(ctx, endpoint, params, "FHPST01780000")
	if err != nil {
		return nil, err
	}
	var result struct {
		Output  []DisparityRankItem `json:"output"`
		MsgCode string              `json:"msg_cd"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse disparity rank: %w", err)
	}
	if result.Output == nil {
		return []DisparityRankItem{}, nil
	}
	return result.Output, nil
}

// GetMarketHolidayInfo checks whether the given date is a business day (CTCA0903R).
// date format: "20060102". Returns the first output entry for the requested date.
func (c *Client) GetMarketHolidayInfo(ctx context.Context, date string) (*HolidayInfo, error) {
	endpoint := "/uapi/domestic-stock/v1/quotations/chk-holiday"
	params := fmt.Sprintf("?BASS_DT=%s&CTX_AREA_NK=&CTX_AREA_FK=", date)

	raw, err := c.get(ctx, endpoint, params, "CTCA0903R")
	if err != nil {
		return nil, err
	}

	var result struct {
		Output []HolidayInfo `json:"output"`
		RtCd   string        `json:"rt_cd"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse holiday info: %w", err)
	}
	if len(result.Output) == 0 {
		return nil, fmt.Errorf("holiday info: empty output for %s", date)
	}
	return &result.Output[0], nil
}

// GetCancellableOrders fetches orders that can still be cancelled or amended (TTTC0084R).
// Returns up to 50 orders. Only pending (미체결) orders are included.
func (c *Client) GetCancellableOrders(ctx context.Context) ([]CancellableOrderItem, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-psbl-rvsecncl"
	params := fmt.Sprintf("?CANO=%s&ACNT_PRDT_CD=%s&CTX_AREA_FK100=&CTX_AREA_NK100=&INQR_DVSN_1=0&INQR_DVSN_2=0",
		c.accountNo, c.accountType)

	raw, err := c.get(ctx, endpoint, params, "TTTC0084R")
	if err != nil {
		return nil, err
	}

	var result struct {
		Output  []CancellableOrderItem `json:"output"`
		MsgCode string                 `json:"msg_cd"`
		Msg     string                 `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse cancellable orders: %w", err)
	}
	if result.Output == nil {
		return []CancellableOrderItem{}, nil
	}
	return result.Output, nil
}

// CancelKISOrder submits a cancellation request for an existing order (TTTC0013U).
// krxOrgNo: ord_gno_brno from GetCancellableOrders (KRX_FWDG_ORD_ORGNO).
// kisOrderID: original KIS order number (ORGN_ODNO).
// ordDvsnCd: order type code from the original order (ORD_DVSN).
// ordUnpr: original order price (ORD_UNPR).
func (c *Client) CancelKISOrder(ctx context.Context, krxOrgNo, kisOrderID, ordDvsnCd, ordUnpr string) (*OrderResponse, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	tok, err := c.tokenManager.EnsureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	endpoint := "/uapi/domestic-stock/v1/trading/order-rvsecncl"
	body, _ := json.Marshal(map[string]string{
		"CANO":               c.accountNo,
		"ACNT_PRDT_CD":       c.accountType,
		"KRX_FWDG_ORD_ORGNO": krxOrgNo,
		"ORGN_ODNO":          kisOrderID,
		"ORD_DVSN":           ordDvsnCd,
		"RVSE_CNCL_DVSN_CD":  "02", // 02 = 취소
		"ORD_QTY":            "0",  // QTY_ALL_ORD_YN=Y 사용 시 0으로 설정
		"ORD_UNPR":           ordUnpr,
		"QTY_ALL_ORD_YN":     "Y", // 잔량 전부 취소
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq, tok.AccessToken, "TTTC0013U")
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
		RtCd    string        `json:"rt_cd"`
		MsgCode string        `json:"msg_cd"`
		Msg     string        `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse cancel order response: %w", err)
	}

	if result.RtCd != "0" {
		c.logAPIError(endpoint, result.MsgCode, string(raw))
		return nil, fmt.Errorf("KIS cancel error [%s]: %s", result.MsgCode, result.Msg)
	}

	return &result.Output, nil
}

// GetOrderHistory fetches a single page of order history for the given date range.
// startDate/endDate format: "20060102". Uses TTTC0081R (real) / VTTC0081R (mock).
// Daily queries return at most ~100 records; pagination is not needed.
func (c *Client) GetOrderHistory(ctx context.Context, startDate, endDate string) ([]map[string]any, error) {
	endpoint := "/uapi/domestic-stock/v1/trading/inquire-daily-ccld"

	q := url.Values{}
	q.Set("CANO", c.accountNo)
	q.Set("ACNT_PRDT_CD", c.accountType)
	q.Set("INQR_STRT_DT", startDate)
	q.Set("INQR_END_DT", endDate)
	q.Set("SLL_BUY_DVSN_CD", "00")
	q.Set("INQR_DVSN", "00")
	q.Set("INQR_DVSN_1", "")
	q.Set("INQR_DVSN_3", "00")
	q.Set("PDNO", "")
	q.Set("CCLD_DVSN", "00")
	q.Set("ORD_GNO_BRNO", "")
	q.Set("ODNO", "")
	q.Set("EXCG_ID_DVSN_CD", "ALL")
	q.Set("CTX_AREA_FK100", "")
	q.Set("CTX_AREA_NK100", "")

	raw, err := c.get(ctx, endpoint, "?"+q.Encode(), "TTTC0081R")
	if err != nil {
		return nil, err
	}

	var result struct {
		Output1 []map[string]any `json:"output1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse order history: %w", err)
	}

	return result.Output1, nil
}

// --- Internal helpers ---

func (c *Client) get(ctx context.Context, endpoint, queryParams, trID string) ([]byte, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	tok, err := c.tokenManager.EnsureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint+queryParams, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, tok.AccessToken, trID)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

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

	// KIS GET endpoints return HTTP 200 even for API-level errors (e.g. expired token).
	// Parse rt_cd from the envelope to detect these cases.
	var envelope struct {
		RtCd    string `json:"rt_cd"`
		MsgCode string `json:"msg_cd"`
		Msg     string `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.RtCd == "1" {
		c.logAPIError(endpoint, envelope.MsgCode, string(raw))
		if envelope.MsgCode == "EGW00123" {
			logger.Info("KIS token expired (EGW00123) — triggering immediate refresh", nil)
			if _, refreshErr := c.tokenManager.IssueToken(ctx); refreshErr != nil {
				logger.Error("immediate token refresh failed", map[string]any{"error": refreshErr.Error()})
			}
		}
		return nil, fmt.Errorf("KIS error [%s]: %s", envelope.MsgCode, envelope.Msg)
	}

	return raw, nil
}

func (c *Client) placeOrder(ctx context.Context, req OrderRequest, trID, endpoint string) (*OrderResponse, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	tok, err := c.tokenManager.EnsureToken(ctx)
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
