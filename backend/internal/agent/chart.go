package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/kis"
)

// Candle holds a single OHLCV candlestick bar.
type Candle struct {
	Date   string  `json:"date"`
	Time   string  `json:"time,omitempty"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
}

// GetChart returns OHLCV candlestick data for the given interval.
//
//   - "1m": 1-minute bars, up to 30 bars (~30 min of intraday data, 1 API call)
//   - "5m": 5-minute bars, up to 78 bars (장 전체 6.5시간 커버, aggregated from 1m; up to 15 calls)
//   - "1h": hourly bars, today's full session (~7 bars; aggregated from 1m; up to 14 calls)
func GetChart(ctx context.Context, client *kis.Client, stockCode, interval string) ([]Candle, error) {
	switch interval {
	case "1m":
		bars, err := fetchMinuteBars(ctx, client, stockCode, 30)
		if err != nil {
			return nil, err
		}
		return minuteBarsToCandles(bars), nil
	case "5m":
		// 390분 = 장 전체(09:00~15:30) 커버. 5분봉 최대 78개.
		bars, err := fetchMinuteBars(ctx, client, stockCode, 390)
		if err != nil {
			return nil, err
		}
		return aggregateMinuteBars(bars, 5), nil
	case "1h":
		// 390 1-minute bars covers a full 6.5-hour trading session (09:00–15:30).
		bars, err := fetchMinuteBars(ctx, client, stockCode, 390)
		if err != nil {
			return nil, err
		}
		return aggregateMinuteBars(bars, 60), nil
	default:
		return nil, fmt.Errorf("unsupported interval %q: use 1m, 5m, or 1h", interval)
	}
}

// fetchMinuteBars fetches 1-minute intraday bars by paginating backward through time.
// Returns bars in ascending time order (oldest → newest).
func fetchMinuteBars(ctx context.Context, client *kis.Client, stockCode string, need int) ([]kis.ChartBar, error) {
	var all []kis.ChartBar
	seen := make(map[string]struct{}) // dedup by date+time key
	refTime := time.Now().Format("150405")

	maxPages := need/30 + 2
	if maxPages > 20 {
		maxPages = 20
	}

	for i := 0; i < maxPages && len(all) < need; i++ {
		pageBars, err := client.GetMinuteChart(ctx, stockCode, refTime)
		if err != nil {
			return nil, err
		}
		if len(pageBars) == 0 {
			break
		}

		newCount := 0
		for _, b := range pageBars {
			key := b.Date + b.Time
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			all = append(all, b)
			newCount++
		}
		if newCount == 0 {
			break // no new bars; reached the beginning of session data
		}

		// Advance reference time to 1 minute before the oldest bar on this page.
		oldest := pageBars[len(pageBars)-1]
		t, err := time.Parse("150405", oldest.Time)
		if err != nil {
			break
		}
		refTime = t.Add(-time.Minute).Format("150405")
	}

	if len(all) == 0 {
		return all, nil
	}

	// KIS returns bars newest-first; reverse to ascending order.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}

// aggregateMinuteBars groups 1-minute bars into bars of intervalMin minutes.
// Input must be in ascending time order (oldest first).
func aggregateMinuteBars(bars []kis.ChartBar, intervalMin int) []Candle {
	if len(bars) == 0 {
		return []Candle{}
	}

	var result []Candle
	var cur *Candle
	curBucket := -1

	for _, b := range bars {
		if len(b.Time) < 4 {
			continue
		}
		h, _ := strconv.Atoi(b.Time[0:2])
		m, _ := strconv.Atoi(b.Time[2:4])
		bucket := (h*60 + m) / intervalMin

		openVal, _ := strconv.ParseFloat(b.Open, 64)
		highVal, _ := strconv.ParseFloat(b.High, 64)
		lowVal, _ := strconv.ParseFloat(b.Low, 64)
		closeVal, _ := strconv.ParseFloat(b.Close, 64)
		volVal, _ := strconv.ParseInt(b.Volume, 10, 64)

		if cur == nil || bucket != curBucket {
			if cur != nil {
				result = append(result, *cur)
			}
			bucketMin := bucket * intervalMin
			barTime := fmt.Sprintf("%02d%02d00", bucketMin/60, bucketMin%60)
			cur = &Candle{
				Date:   b.Date,
				Time:   barTime,
				Open:   openVal,
				High:   highVal,
				Low:    lowVal,
				Close:  closeVal,
				Volume: volVal,
			}
			curBucket = bucket
		} else {
			if highVal > cur.High {
				cur.High = highVal
			}
			if lowVal < cur.Low {
				cur.Low = lowVal
			}
			cur.Close = closeVal
			cur.Volume += volVal
		}
	}
	if cur != nil {
		result = append(result, *cur)
	}
	return result
}

// minuteBarsToCandles converts raw KIS ChartBar slices to Candle slices.
func minuteBarsToCandles(bars []kis.ChartBar) []Candle {
	result := make([]Candle, 0, len(bars))
	for _, b := range bars {
		openVal, _ := strconv.ParseFloat(b.Open, 64)
		highVal, _ := strconv.ParseFloat(b.High, 64)
		lowVal, _ := strconv.ParseFloat(b.Low, 64)
		closeVal, _ := strconv.ParseFloat(b.Close, 64)
		volVal, _ := strconv.ParseInt(b.Volume, 10, 64)
		result = append(result, Candle{
			Date:   b.Date,
			Time:   b.Time,
			Open:   openVal,
			High:   highVal,
			Low:    lowVal,
			Close:  closeVal,
			Volume: volVal,
		})
	}
	return result
}
