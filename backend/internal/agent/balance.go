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
//   - TradableAmount    : 거래가능금액 (dnca_tot_amt / 예수금총금액) — 오늘 주식 매수에 사용 가능한 금액
//   - WithdrawableAmount: 출금가능금액 (prvs_rcdl_excc_amt / D+2 정산금액) — 실제 출금 가능한 금액
type AccountBalance struct {
	TotalEval          float64 `json:"total_eval"`
	TradableAmount     float64 `json:"tradable_amount"`     // 거래가능금액 (에이전트 매수 판단 기준)
	WithdrawableAmount float64 `json:"withdrawable_amount"` // 출금가능금액 (D+2)
	PurchaseAmt        float64 `json:"purchase_amt"`
	EvalProfitLoss     float64 `json:"eval_profit_loss"`
	ProfitRate         string  `json:"profit_rate"`
}

// GetAccountBalance fetches account balance from inquire-balance (단일 호출):
//   - output2 → 총평가금액, 거래가능금액(dnca_tot_amt), 출금가능금액(prvs_rcdl_excc_amt), 평가손익
func GetAccountBalance(ctx context.Context, client *kis.Client, db *database.DB) (*AccountBalance, error) {
	summary, err := client.GetInquireBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetInquireBalance: %w", err)
	}

	totalEval, _ := strconv.ParseFloat(summary.TotalEval, 64)
	tradable, _ := strconv.ParseFloat(summary.DepositAmt, 64)          // dnca_tot_amt = 거래가능금액
	withdrawable, _ := strconv.ParseFloat(summary.WithdrawableAmt, 64) // prvs_rcdl_excc_amt = 출금가능금액
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
