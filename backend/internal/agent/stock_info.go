package agent

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/kis"
)

// StockInfo holds key stock data for the AI agent's decision-making,
// including current price and moving averages derived from daily closes.
type StockInfo struct {
	StockCode    string  `json:"stock_code"`
	CurrentPrice string  `json:"current_price"`
	ChangeRate   string  `json:"change_rate"`
	Volume       string  `json:"volume"`
	MA5          float64 `json:"ma5"`
	MA20         float64 `json:"ma20"`
}

// GetStockInfo fetches the latest price and MA5/MA20 for the given stock code.
// MA values are derived from the last ~40 calendar days of daily closing prices.
// If daily data is unavailable, MA5 and MA20 are returned as 0.
func GetStockInfo(ctx context.Context, client *kis.Client, stockCode string) (*StockInfo, error) {
	if stockCode == "" {
		return nil, fmt.Errorf("stock_code is required")
	}

	resp, err := client.GetStockPrice(ctx, stockCode)
	if err != nil {
		return nil, fmt.Errorf("GetStockInfo [%s]: %w", stockCode, err)
	}

	info := &StockInfo{
		StockCode:    resp.StockCode,
		CurrentPrice: resp.CurrentPrice,
		ChangeRate:   resp.ChangeRate,
		Volume:       resp.Volume,
	}

	// Fetch ~40 calendar days to guarantee at least 20 trading days for MA20.
	endDate := time.Now().Format("20060102")
	startDate := time.Now().AddDate(0, 0, -40).Format("20060102")
	daily, maErr := client.GetDailyChart(ctx, stockCode, startDate, endDate)
	if maErr == nil && len(daily) > 0 {
		// KIS returns bars newest-first; reverse to build closes in ascending order.
		closes := make([]float64, 0, len(daily))
		for i := len(daily) - 1; i >= 0; i-- {
			v, parseErr := strconv.ParseFloat(daily[i].Close, 64)
			if parseErr == nil && v > 0 {
				closes = append(closes, v)
			}
		}
		info.MA5 = calcMA(closes, 5)
		info.MA20 = calcMA(closes, 20)
	}

	return info, nil
}

// calcMA returns the simple moving average of the last `period` values in closes.
// Returns 0 if there are fewer than `period` values (insufficient data).
func calcMA(closes []float64, period int) float64 {
	if len(closes) < period {
		return 0
	}
	sum := 0.0
	for _, v := range closes[len(closes)-period:] {
		sum += v
	}
	return math.Round(sum/float64(period)*100) / 100
}
