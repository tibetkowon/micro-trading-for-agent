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
		excludeStr = "(없음)"
	}

	prompt := fmt.Sprintf(`당신은 한국 주식 단타 트레이딩 AI입니다.

아래 순위 데이터를 분석하여 매수할 종목 1개를 선정하세요.

**조건:**
- 가용자금: %.0f원
- 제외 종목 (오늘 이미 거래함): %s
- 목표: 단기(당일) 수익 극대화

**순위 데이터 (JSON):**
%s

**응답 형식 (반드시 JSON만 출력):**
{"stock_code":"종목코드6자리","reason":"선정 이유 (한국어, 2문장 이내)"}`,
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

// GenerateReport asks Claude to write a Korean markdown daily trading report.
func (c *ClaudeClient) GenerateReport(
	ctx context.Context,
	date string,
	trades []reportOrder,
	totalEval float64,
	withdrawable float64,
) (string, error) {
	tradesJSON, _ := json.Marshal(trades)

	prompt := fmt.Sprintf(`당신은 한국 주식 자동매매 시스템의 일일 리포트 작성 AI입니다.

아래 데이터를 바탕으로 %s의 일일 트레이딩 리포트를 한국어 마크다운으로 작성하세요.

**계좌 현황:**
- 총 평가금액: %.0f원
- 출금가능금액: %.0f원

**오늘의 거래 내역 (JSON):**
%s

**작성 지침:**
- ## 헤더로 섹션 구분
- 종목별 매수/매도 결과, 손익, 수익률 정리
- 전체 성과 요약 (총 실현 손익, 승률 등)
- 내일을 위한 간단한 분석 및 조언
- 간결하게 300자 이내`,
		date, totalEval, withdrawable, string(tradesJSON))

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 1024,
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

// reportOrder is a simplified order view for the report prompt.
type reportOrder struct {
	StockCode   string  `json:"stock_code"`
	StockName   string  `json:"stock_name"`
	OrderType   string  `json:"order_type"`
	Qty         int     `json:"qty"`
	Price       float64 `json:"price"`
	FilledPrice float64 `json:"filled_price"`
	Status      string  `json:"status"`
}
