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
//
//   - TradableAmount    : 거래가능금액 (nrcvb_buy_amt / 미수없는매수금액) — KIS 앱 "주문가능"과 동일
//   - WithdrawableAmount: 출금가능금액 (dnca_tot_amt / 예수금총금액) — KIS 앱 "출금가능"과 동일
type AccountBalance struct {
	TotalEval          float64 `json:"total_eval"`
	TradableAmount     float64 `json:"tradable_amount"`     // 거래가능금액 (에이전트 매수 판단 기준)
	WithdrawableAmount float64 `json:"withdrawable_amount"` // 출금가능금액 (D+2)
	PurchaseAmt        float64 `json:"purchase_amt"`
	EvalProfitLoss     float64 `json:"eval_profit_loss"`
	ProfitRate         string  `json:"profit_rate"`
}

// GetAccountBalance fetches account balance via two KIS API calls:
//   - inquire-balance output2 → 총평가금액, 출금가능금액(dnca_tot_amt/예수금), 평가손익
//   - inquire-psbl-order → 거래가능금액(nrcvb_buy_amt/미수없는매수금액 = KIS 앱 주문가능)
func GetAccountBalance(ctx context.Context, client *kis.Client, db *database.DB) (*AccountBalance, error) {
	summary, err := client.GetInquireBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetInquireBalance: %w", err)
	}

	totalEval, _ := strconv.ParseFloat(summary.TotalEval, 64)
	withdrawable, _ := strconv.ParseFloat(summary.DepositAmt, 64) // dnca_tot_amt = 예수금 = 출금가능금액

	// 거래가능금액: nrcvb_buy_amt (미수없는매수금액) = KIS 앱 "주문가능"
	tradable := withdrawable // fallback
	if avail, err := client.GetAvailableOrder(ctx); err == nil {
		if v, err2 := strconv.ParseFloat(avail.AvailableAmount, 64); err2 == nil && v > 0 {
			tradable = v
		}
	}
	purchaseAmt, _ := strconv.ParseFloat(summary.PurchaseAmt, 64)
	evalProfitLoss, _ := strconv.ParseFloat(summary.EvalProfitLoss, 64)

	// 수익률 계산: 매입금액이 0이면 "-"
	profitRate := "-"
	if purchaseAmt > 0 {
		rate := evalProfitLoss / purchaseAmt * 100
		profitRate = fmt.Sprintf("%.2f", rate)
	}

	// DB 스냅샷 (tradable_amount 기준 저장)
	_, _ = db.ExecContext(ctx,
		`INSERT INTO balances (total_eval, available_amount) VALUES (?, ?)`,
		totalEval, tradable,
	)

	return &AccountBalance{
		TotalEval:          totalEval,
		TradableAmount:     tradable,
		WithdrawableAmount: withdrawable,
		PurchaseAmt:        purchaseAmt,
		EvalProfitLoss:     evalProfitLoss,
		ProfitRate:         profitRate,
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
