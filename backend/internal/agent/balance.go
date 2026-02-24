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
	ProfitRate      string  `json:"profit_rate"`
}

// GetAccountBalance fetches account balance from KIS and snapshots it to DB.
func GetAccountBalance(ctx context.Context, client *kis.Client, db *database.DB) (*AccountBalance, error) {
	resp, err := client.GetBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetAccountBalance: %w", err)
	}

	totalEval, _ := strconv.ParseFloat(resp.TotalEval, 64)
	available, _ := strconv.ParseFloat(resp.AvailableAmount, 64)

	// Snapshot to DB for historical tracking
	_, dbErr := db.ExecContext(ctx,
		`INSERT INTO balances (total_eval, available_amount) VALUES (?, ?)`,
		totalEval, available,
	)
	if dbErr != nil {
		// Non-fatal: log but don't block the agent
		_ = dbErr
	}

	return &AccountBalance{
		TotalEval:       totalEval,
		AvailableAmount: available,
		ProfitRate:      resp.ProfitRate,
	}, nil
}

// GetLatestBalanceFromDB returns the most recent balance snapshot from the database.
// Useful when KIS API is temporarily unavailable.
func GetLatestBalanceFromDB(ctx context.Context, db *database.DB) (*models.Balance, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, total_eval, available_amount, recorded_at FROM balances ORDER BY id DESC LIMIT 1`)

	var b models.Balance
	if err := row.Scan(&b.ID, &b.TotalEval, &b.AvailableAmount, &b.RecordedAt); err != nil {
		return nil, fmt.Errorf("no balance snapshot: %w", err)
	}
	return &b, nil
}
