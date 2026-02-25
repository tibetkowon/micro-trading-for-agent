package agent

import (
	"context"
	"fmt"
	"strconv"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

// AccountBalance holds the parsed account balance for dashboard display.
// Source: TTTC8434R (inquire-balance) — single API call, no 주문가능금액.
//
//   - WithdrawableAmount: 출금가능금액 (dnca_tot_amt / 예수금) — KIS 앱 "출금가능"과 동일
//   - AssetChangeAmt    : 자산증감액 (asst_icdc_amt) — 전일 대비 자산 변동금액
//   - AssetChangeRate   : 자산증감수익률 (계산값: asst_icdc_amt / bfdy_tot_asst_evlu_amt × 100)
type AccountBalance struct {
	TotalEval          float64 `json:"total_eval"`
	WithdrawableAmount float64 `json:"withdrawable_amount"` // 출금가능금액 (dnca_tot_amt)
	AssetChangeAmt     float64 `json:"asset_change_amt"`    // 자산증감액
	AssetChangeRate    string  `json:"asset_change_rate"`   // 자산증감수익률 (%)
}

// GetAccountBalance fetches account balance via TTTC8434R (inquire-balance output2).
// 주문가능금액은 대시보드에서 제거됨 — 에이전트 주문 시 TTTC8908R로 종목별 확인.
func GetAccountBalance(ctx context.Context, client *kis.Client, db *database.DB) (*AccountBalance, error) {
	summary, err := client.GetInquireBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetInquireBalance: %w", err)
	}

	totalEval, _ := strconv.ParseFloat(summary.TotalEval, 64)
	withdrawable, _ := strconv.ParseFloat(summary.DepositAmt, 64)     // dnca_tot_amt = 출금가능금액
	assetChangeAmt, _ := strconv.ParseFloat(summary.AssetChangeAmt, 64)
	prevTotal, _ := strconv.ParseFloat(summary.PrevTotalAsset, 64)

	// asst_icdc_erng_rt는 KIS가 "데이터 미제공" (항상 0) — 직접 계산
	assetChangeRate := "-"
	if prevTotal > 0 {
		rate := assetChangeAmt / prevTotal * 100
		assetChangeRate = fmt.Sprintf("%.2f", rate)
	}

	// DB 스냅샷
	_, _ = db.ExecContext(ctx,
		`INSERT INTO balances (total_eval, available_amount) VALUES (?, ?)`,
		totalEval, withdrawable,
	)

	return &AccountBalance{
		TotalEval:          totalEval,
		WithdrawableAmount: withdrawable,
		AssetChangeAmt:     assetChangeAmt,
		AssetChangeRate:    assetChangeRate,
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
