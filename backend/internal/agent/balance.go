package agent

import (
	"context"
	"fmt"
	"strconv"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

// AccountBalance holds the parsed account balance for agent decisions.
type AccountBalance struct {
	TotalEval       float64 `json:"total_eval"`
	AvailableAmount float64 `json:"available_amount"`
	PurchaseAmt     float64 `json:"purchase_amt"`
	EvalProfitLoss  float64 `json:"eval_profit_loss"`
	ProfitRate      string  `json:"profit_rate"`
}

// GetAccountBalance fetches account balance using two KIS endpoints:
//   - inquire-balance   → 총평가금액, 예수금, 평가손익
//   - inquire-psbl-order → 주문가능현금
func GetAccountBalance(ctx context.Context, client *kis.Client, db *database.DB) (*AccountBalance, error) {
	// 1. 주식잔고조회: 총평가금액, 평가손익
	summary, err := client.GetInquireBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetInquireBalance: %w", err)
	}

	// 2. 매수가능조회: 주문가능현금
	avail, err := client.GetAvailableOrder(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetAvailableOrder: %w", err)
	}

	totalEval, _ := strconv.ParseFloat(summary.TotalEval, 64)
	purchaseAmt, _ := strconv.ParseFloat(summary.PurchaseAmt, 64)
	evalProfitLoss, _ := strconv.ParseFloat(summary.EvalProfitLoss, 64)
	available, _ := strconv.ParseFloat(avail.AvailableAmount, 64)

	// 수익률 계산: 매입금액이 0이면 "-"
	profitRate := "-"
	if purchaseAmt > 0 {
		rate := evalProfitLoss / purchaseAmt * 100
		profitRate = fmt.Sprintf("%.2f", rate)
	}

	// DB 스냅샷
	_, _ = db.ExecContext(ctx,
		`INSERT INTO balances (total_eval, available_amount) VALUES (?, ?)`,
		totalEval, available,
	)

	return &AccountBalance{
		TotalEval:       totalEval,
		AvailableAmount: available,
		PurchaseAmt:     purchaseAmt,
		EvalProfitLoss:  evalProfitLoss,
		ProfitRate:      profitRate,
	}, nil
}

// GetLatestBalanceFromDB returns the most recent balance snapshot from the database.
func GetLatestBalanceFromDB(ctx context.Context, db *database.DB) (*models.Balance, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, total_eval, available_amount, recorded_at FROM balances ORDER BY id DESC LIMIT 1`)

	var b models.Balance
	if err := row.Scan(&b.ID, &b.TotalEval, &b.AvailableAmount, &b.RecordedAt); err != nil {
		return nil, fmt.Errorf("no balance snapshot: %w", err)
	}
	return &b, nil
}
