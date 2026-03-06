package trader

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// RankItem is a unified representation of a stock from any ranking API,
// enriched with technical indicators from GetStockInfo.
type RankItem struct {
	DataRank     string `json:"data_rank"`
	StockCode    string `json:"stock_code"`
	StockName    string `json:"stock_name"`
	CurrentPrice string `json:"current_price"`
	Volume       string `json:"volume"`
	RankingType  string `json:"ranking_type"`            // e.g. "volume+strength"
	VolIncrRate  string `json:"vol_incr_rate,omitempty"` // 거래량 증가율 % (volume)
	Strength     string `json:"strength,omitempty"`      // 체결강도 % (strength)
	NetBuyQty    string `json:"net_buy_qty,omitempty"`   // 순매수체결량 (exec_count)
	DisparityD20 string `json:"disparity_d20,omitempty"` // 20일 이격도 (disparity)
	// Technical indicators from GetStockInfo
	MA5        float64 `json:"ma5,omitempty"`
	MA20       float64 `json:"ma20,omitempty"`
	RSI14      float64 `json:"rsi14,omitempty"`
	MACDLine   float64 `json:"macd_line,omitempty"`
	MACDSignal float64 `json:"macd_signal,omitempty"`
}

// ClaudeClient wraps the Anthropic API for trading decisions.
type ClaudeClient struct {
	client anthropic.Client
	model  string
}

// NewClaudeClient creates a ClaudeClient with the given API key and model.
func NewClaudeClient(apiKey, model string) *ClaudeClient {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &ClaudeClient{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

// StockCandidate is one entry in Claude's ranked selection list.
type StockCandidate struct {
	StockCode string `json:"stock_code"`
	Reason    string `json:"reason"`
}

// SelectStocks asks Claude to rank all viable candidates from the ranking list.
// Returns an ordered slice — index 0 is the top pick. Engine tries them in order.
func (c *ClaudeClient) SelectStocks(
	ctx context.Context,
	rankings []RankItem,
	availableCash float64,
	excludedCodes []string,
) ([]StockCandidate, error) {
	if len(rankings) == 0 {
		return nil, fmt.Errorf("ranking list is empty")
	}

	rankJSON, _ := json.Marshal(rankings)
	excludeStr := strings.Join(excludedCodes, ", ")
	if excludeStr == "" {
		excludeStr = "none"
	}

	prompt := fmt.Sprintf(`You are a Korean stock intraday trading AI.

Analyze the ranking data below and rank ALL viable stocks for same-day trading, best first.
Exclude any stock in the excluded list.

Constraints:
- Available cash: %.0f KRW
- Excluded stocks (already traded today): %s
- Goal: maximize intraday return
- Consider: MA trend (price vs MA5/MA20), RSI (avoid >70), MACD direction, volume momentum

Ranking data (JSON):
%s

Respond with ONLY a JSON array — no explanation, no markdown.
Order from best to worst. Include only stocks worth buying (skip clearly bad ones):
[{"stock_code":"6-digit code","reason":"한국어로 1문장"},...]`,
		availableCash, excludeStr, string(rankJSON))

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 512,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("claude SelectStocks API error: %w", err)
	}

	if len(msg.Content) == 0 {
		return nil, fmt.Errorf("claude returned empty response")
	}

	raw := strings.TrimSpace(msg.Content[0].AsText().Text)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 {
			raw = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var candidates []StockCandidate
	if err := json.Unmarshal([]byte(raw), &candidates); err != nil {
		return nil, fmt.Errorf("claude response parse error: %w (raw: %s)", err, raw)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("claude returned empty candidate list")
	}

	return candidates, nil
}

// ReportSummary holds pre-computed trading stats passed to GenerateReport.
type ReportSummary struct {
	Date         string
	TotalPnL     float64 // total realized profit/loss in KRW
	WinCount     int
	LossCount    int
	TotalEval    float64
	Withdrawable float64
	TradeTable   string // pre-rendered markdown table
}

// GenerateReport asks Claude to write a short Korean analysis based on pre-computed stats.
// The markdown table is already built by the engine; Claude only writes the analysis section.
func (c *ClaudeClient) GenerateReport(ctx context.Context, s ReportSummary) (string, error) {
	winRate := 0.0
	total := s.WinCount + s.LossCount
	if total > 0 {
		winRate = float64(s.WinCount) / float64(total) * 100
	}

	prompt := fmt.Sprintf(`You are a Korean stock auto-trading system analyst.

Based on the trading stats below, write a SHORT Korean analysis (3~5 sentences max).
Do NOT reproduce the table. Focus on: overall assessment, what worked or didn't, and one actionable tip for tomorrow.
Write in Korean.

Date: %s
Total realized P&L: %.0f KRW
Win/Loss: %d wins / %d losses (win rate %.1f%%)
Account total eval: %.0f KRW
Withdrawable: %.0f KRW`,
		s.Date, s.TotalPnL, s.WinCount, s.LossCount, winRate, s.TotalEval, s.Withdrawable)

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 512,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude GenerateReport API error: %w", err)
	}

	if len(msg.Content) == 0 {
		return "", fmt.Errorf("claude returned empty report")
	}

	return msg.Content[0].AsText().Text, nil
}
