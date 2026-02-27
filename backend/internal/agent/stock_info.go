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
// including current price, moving averages, trading value, RSI, and MACD.
type StockInfo struct {
	StockCode    string  `json:"stock_code"`
	CurrentPrice string  `json:"current_price"`
	ChangeRate   string  `json:"change_rate"`
	Volume       string  `json:"volume"`
	TradingValue float64 `json:"trading_value"` // 거래대금 (volume × price, KRW); 0 = unavailable
	MA5          float64 `json:"ma5"`
	MA20         float64 `json:"ma20"`
	RSI14        float64 `json:"rsi14"`          // RSI(14) from 5-minute closes; 0 = insufficient data
	MACDLine     float64 `json:"macd_line"`      // MACD line (EMA12 − EMA26) from 5m candles
	MACDSignal   float64 `json:"macd_signal"`    // Signal line (EMA9 of MACD line) from 5m candles
	MACDHisto    float64 `json:"macd_histogram"` // Histogram (MACD line − Signal line)
}

// GetStockInfo fetches the latest price and computes all technical indicators:
//   - MA5 / MA20: from daily closes (~40 calendar days)
//   - TradingValue: current_price × today's volume (KRW)
//   - RSI(14), MACD(12,26,9): from 5-minute candles (up to 200 1-min bars → ~40 5-min bars)
//
// Indicator fields are 0 when there is insufficient market data (pre-open, thin session, etc.).
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

	// --- TradingValue: current_price × today's volume ---
	price, _ := strconv.ParseFloat(resp.CurrentPrice, 64)
	vol, _ := strconv.ParseFloat(resp.Volume, 64)
	if price > 0 && vol > 0 {
		info.TradingValue = math.Round(price * vol)
	}

	// --- MA5 / MA20 from daily closes ---
	endDate := time.Now().Format("20060102")
	startDate := time.Now().AddDate(0, 0, -40).Format("20060102")
	daily, maErr := client.GetDailyChart(ctx, stockCode, startDate, endDate)
	if maErr == nil && len(daily) > 0 {
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

	// --- RSI(14) and MACD(12,26,9) from 5-minute candles ---
	// Fetch 200 1-minute bars → aggregate to ~40 5-minute bars.
	// 40 bars is sufficient for MACD(12,26,9) which needs 26+9-1 = 34 periods minimum.
	bars, chartErr := fetchMinuteBars(ctx, client, stockCode, 200)
	if chartErr == nil && len(bars) > 0 {
		candles5m := aggregateMinuteBars(bars, 5)
		if len(candles5m) >= 2 {
			closes5m := make([]float64, len(candles5m))
			for i, c := range candles5m {
				closes5m[i] = c.Close
			}
			info.RSI14 = calcRSI(closes5m, 14)
			info.MACDLine, info.MACDSignal, info.MACDHisto = calcMACD(closes5m, 12, 26, 9)
		}
	}

	return info, nil
}

// --- Moving Average ---

// calcMA returns the simple moving average of the last `period` values in closes.
// Returns 0 if there are fewer than `period` values.
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

// --- RSI ---

// calcRSI computes Wilder's RSI for the given period.
// Returns 0 when len(closes) < period+1 (insufficient data).
func calcRSI(closes []float64, period int) float64 {
	if len(closes) < period+1 {
		return 0
	}

	// Seed: initial average gain / average loss over first `period` changes.
	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		delta := closes[i] - closes[i-1]
		if delta > 0 {
			avgGain += delta
		} else {
			avgLoss += -delta
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	// Wilder smoothing for subsequent bars.
	for i := period + 1; i < len(closes); i++ {
		delta := closes[i] - closes[i-1]
		gain, loss := 0.0, 0.0
		if delta > 0 {
			gain = delta
		} else {
			loss = -delta
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
	}

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return math.Round((100-100/(1+rs))*100) / 100
}

// --- EMA / MACD ---

// calcEMA returns the full EMA series for the given period.
// The first period-1 entries are 0 (seeding phase); valid values start at index period-1.
// Returns nil when len(closes) < period.
func calcEMA(closes []float64, period int) []float64 {
	if len(closes) < period {
		return nil
	}
	k := 2.0 / float64(period+1)
	ema := make([]float64, len(closes))

	// Seed: SMA of first `period` closes.
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	ema[period-1] = sum / float64(period)

	for i := period; i < len(closes); i++ {
		ema[i] = closes[i]*k + ema[i-1]*(1-k)
	}
	return ema
}

// calcMACD returns the last MACD line, signal, and histogram values.
// Standard parameters: fastPeriod=12, slowPeriod=26, signalPeriod=9.
// Returns (0, 0, 0) when data is insufficient.
func calcMACD(closes []float64, fastPeriod, slowPeriod, signalPeriod int) (macdLine, signal, histogram float64) {
	if len(closes) < slowPeriod {
		return 0, 0, 0
	}

	fastEMA := calcEMA(closes, fastPeriod)
	slowEMA := calcEMA(closes, slowPeriod)
	if fastEMA == nil || slowEMA == nil {
		return 0, 0, 0
	}

	// MACD series: valid from index slowPeriod-1 onward.
	validLen := len(closes) - slowPeriod + 1
	macdSeries := make([]float64, validLen)
	for i := 0; i < validLen; i++ {
		idx := i + slowPeriod - 1
		macdSeries[i] = fastEMA[idx] - slowEMA[idx]
	}

	lastMACD := macdSeries[len(macdSeries)-1]

	if len(macdSeries) < signalPeriod {
		// Not enough MACD values for the signal EMA yet.
		return math.Round(lastMACD*100) / 100, 0, 0
	}

	signalEMA := calcEMA(macdSeries, signalPeriod)
	if signalEMA == nil {
		return math.Round(lastMACD*100) / 100, 0, 0
	}

	lastSignal := signalEMA[len(signalEMA)-1]
	lastHisto := lastMACD - lastSignal

	return math.Round(lastMACD*100) / 100,
		math.Round(lastSignal*100) / 100,
		math.Round(lastHisto*100) / 100
}
