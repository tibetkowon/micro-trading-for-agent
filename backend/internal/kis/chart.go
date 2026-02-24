package kis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ChartBar holds one OHLCV bar returned from the KIS chart APIs.
type ChartBar struct {
	Date   string // YYYYMMDD
	Time   string // HHMMSS (empty for daily bars)
	Open   string
	High   string
	Low    string
	Close  string
	Volume string
}

// GetMinuteChart fetches up to 30 intraday 1-minute bars ending at or before
// inputHour (HHMMSS format). Pass "" to use the current time.
// Bars are returned newest-first.
func (c *Client) GetMinuteChart(ctx context.Context, stockCode, inputHour string) ([]ChartBar, error) {
	if inputHour == "" {
		inputHour = time.Now().Format("150405")
	}
	endpoint := "/uapi/domestic-stock/v1/quotations/inquire-time-itemchartprice"
	params := fmt.Sprintf(
		"?FID_COND_MRKT_DIV_CODE=J&FID_ETC_CLS_CODE=0&FID_INPUT_ISCD=%s&FID_INPUT_HOUR_1=%s&FID_PW_DATA_INCU_YN=N",
		stockCode, inputHour,
	)

	raw, err := c.get(ctx, endpoint, params, "FHKST03010200")
	if err != nil {
		return nil, err
	}

	var result struct {
		Output2 []struct {
			Date   string `json:"stck_bsop_date"`
			Time   string `json:"stck_cntg_hour"`
			Open   string `json:"stck_oprc"`
			High   string `json:"stck_hgpr"`
			Low    string `json:"stck_lwpr"`
			Close  string `json:"stck_prpr"`
			Volume string `json:"cntg_vol"`
		} `json:"output2"`
		MsgCode string `json:"msg_cd"`
		Msg     string `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse minute chart: %w", err)
	}

	bars := make([]ChartBar, 0, len(result.Output2))
	for _, o := range result.Output2 {
		bars = append(bars, ChartBar{
			Date:   o.Date,
			Time:   o.Time,
			Open:   o.Open,
			High:   o.High,
			Low:    o.Low,
			Close:  o.Close,
			Volume: o.Volume,
		})
	}
	return bars, nil
}

// GetDailyChart fetches daily OHLCV bars between startDate and endDate (YYYYMMDD).
// Returns bars newest-first.
func (c *Client) GetDailyChart(ctx context.Context, stockCode, startDate, endDate string) ([]ChartBar, error) {
	endpoint := "/uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice"
	params := fmt.Sprintf(
		"?FID_COND_MRKT_DIV_CODE=J&FID_INPUT_ISCD=%s&FID_INPUT_DATE_1=%s&FID_INPUT_DATE_2=%s&FID_PERIOD_DIV_CODE=D&FID_ORG_ADJ_PRC=0",
		stockCode, startDate, endDate,
	)

	raw, err := c.get(ctx, endpoint, params, "FHKST03010100")
	if err != nil {
		return nil, err
	}

	var result struct {
		Output2 []struct {
			Date   string `json:"stck_bsop_date"`
			Open   string `json:"stck_oprc"`
			High   string `json:"stck_hgpr"`
			Low    string `json:"stck_lwpr"`
			Close  string `json:"stck_clpr"` // 일봉 종가는 stck_clpr
			Volume string `json:"acml_vol"`
		} `json:"output2"`
		MsgCode string `json:"msg_cd"`
		Msg     string `json:"msg1"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		c.logAPIError(endpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse daily chart: %w", err)
	}

	bars := make([]ChartBar, 0, len(result.Output2))
	for _, o := range result.Output2 {
		bars = append(bars, ChartBar{
			Date:   o.Date,
			Open:   o.Open,
			High:   o.High,
			Low:    o.Low,
			Close:  o.Close,
			Volume: o.Volume,
		})
	}
	return bars, nil
}
