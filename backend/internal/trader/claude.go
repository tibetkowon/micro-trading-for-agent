package trader

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// RankItem is a unified representation of a stock from any ranking API.
type RankItem struct {
	DataRank     string `json:"data_rank"`
	StockCode    string `json:"stock_code"`
	StockName    string `json:"stock_name"`
	CurrentPrice string `json:"current_price"`
	Volume       string `json:"volume"`
	RankingType  string `json:"ranking_type"` // volume, strength, exec_count, disparity
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

type selectStockResponse struct {
	StockCode string `json:"stock_code"`
	Reason    string `json:"reason"`
}

// SelectStock asks Claude to pick the best stock from the ranking list.
// Returns the selected stock code, reason, and any error.
func (c *ClaudeClient) SelectStock(
	ctx context.Context,
	rankings []RankItem,
	availableCash float64,
	excludedCodes []string,
) (stockCode, reason string, err error) {
	if len(rankings) == 0 {
		return "", "", fmt.Errorf("ranking list is empty")
	}

	rankJSON, _ := json.Marshal(rankings)
	excludeStr := strings.Join(excludedCodes, ", ")
	if excludeStr == "" {
		excludeStr = "none"
	}

	prompt := fmt.Sprintf(`You are a Korean stock intraday trading AI.

Analyze the ranking data below and select exactly 1 stock to buy for maximum same-day profit.

Constraints:
- Available cash: %.0f KRW
- Excluded stocks (already traded today): %s
- Goal: maximize intraday return

Ranking data (JSON):
%s

Respond with ONLY a JSON object — no explanation, no markdown:
{"stock_code":"6-digit code","reason":"선정 이유를 한국어로 2문장 이내로 작성"}`,
		availableCash, excludeStr, string(rankJSON))

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 256,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("claude SelectStock API error: %w", err)
	}

	if len(msg.Content) == 0 {
		return "", "", fmt.Errorf("claude returned empty response")
	}

	raw := msg.Content[0].AsText().Text
	raw = strings.TrimSpace(raw)

	// Strip markdown code fence if present
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 {
			raw = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var resp selectStockResponse
	if jsonErr := json.Unmarshal([]byte(raw), &resp); jsonErr != nil {
		return "", "", fmt.Errorf("claude response parse error: %w (raw: %s)", jsonErr, raw)
	}
	if resp.StockCode == "" {
		return "", "", fmt.Errorf("claude returned empty stock_code (raw: %s)", raw)
	}

	return resp.StockCode, resp.Reason, nil
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
