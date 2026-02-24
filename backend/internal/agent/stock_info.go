package agent

import (
	"context"
	"fmt"

	"github.com/micro-trading-for-agent/backend/internal/kis"
)

// StockInfo holds key information about a stock for the AI agent's decision-making.
type StockInfo struct {
	StockCode    string `json:"stock_code"`
	CurrentPrice string `json:"current_price"`
	ChangeRate   string `json:"change_rate"`
	Volume       string `json:"volume"`
}

// GetStockInfo fetches the latest price data for a single stock code.
// This is the primary data input for the AI agent's trade selection logic.
func GetStockInfo(ctx context.Context, client *kis.Client, stockCode string) (*StockInfo, error) {
	if stockCode == "" {
		return nil, fmt.Errorf("stock_code is required")
	}

	resp, err := client.GetStockPrice(ctx, stockCode)
	if err != nil {
		return nil, fmt.Errorf("GetStockInfo [%s]: %w", stockCode, err)
	}

	return &StockInfo{
		StockCode:    resp.StockCode,
		CurrentPrice: resp.CurrentPrice,
		ChangeRate:   resp.ChangeRate,
		Volume:       resp.Volume,
	}, nil
}
