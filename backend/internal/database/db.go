package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

// TradingSettings holds all autonomous trading configuration values.
type TradingSettings struct {
	TakeProfitPct             float64  // 익절 기준 %
	StopLossPct               float64  // 손절 기준 %
	RankingTypes              []string // 순위 유형 우선순위 (volume, strength, exec_count, disparity)
	RankingPriceMin           string   // 순위 조회 최소 주가
	RankingPriceMax           string   // 순위 조회 최대 주가
	MaxPositions              int      // 동시 보유 최대 종목 수
	OrderAmountPct            float64  // 가용자금 대비 주문 비율(%)
	SellConditions            []string // 매도 조건 우선순위 배열
	IndicatorCheckIntervalMin int      // 지표 확인 주기(분)
	IndicatorRSISellThreshold float64  // RSI 매도 기준값
	IndicatorMACDBearishSell  bool     // MACD 데드크로스 매도 여부
	ClaudeModel               string   // 사용할 Claude 모델
	// 순위별 필터
	RankingVolumeMinIncrRate   float64 // 거래량 증가율 최솟값 (0=필터없음)
	RankingStrengthMin         float64 // 체결강도 최솟값 (0=필터없음)
	RankingExecCountNetBuyOnly bool    // 대량체결: 순매수 우세 종목만
	RankingDisparityD20Min     float64 // 20일 이격도 최솟값 (0=필터없음)
	RankingDisparityD20Max     float64 // 20일 이격도 최댓값 (0=필터없음)
}

// DB wraps the sql.DB connection.
type DB struct {
	*sql.DB
}

// New opens (or creates) the SQLite database at the given path,
// runs schema migrations, and returns a ready-to-use DB instance.
func New(dsn string) (*DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dsn)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	sqlDB, err := sql.Open("sqlite3", dsn+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Limit connections to avoid memory pressure on NCP Micro (1 GB RAM).
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// migrate creates all required tables if they do not already exist.
func (db *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS tokens (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			access_token TEXT    NOT NULL,
			issued_at    DATETIME NOT NULL DEFAULT (datetime('now')),
			expires_at   DATETIME NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS orders (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			stock_code   TEXT    NOT NULL,
			stock_name   TEXT    NOT NULL DEFAULT '',
			order_type   TEXT    NOT NULL CHECK(order_type IN ('BUY','SELL')),
			qty          INTEGER NOT NULL CHECK(qty > 0),
			price        REAL    NOT NULL CHECK(price >= 0),
			filled_price REAL    NOT NULL DEFAULT 0,
			status       TEXT    NOT NULL DEFAULT 'PENDING'
			                CHECK(status IN ('PENDING','FILLED','PARTIALLY_FILLED','CANCELLED','FAILED')),
			kis_order_id TEXT    NOT NULL DEFAULT '',
			created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS balances (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			total_eval       REAL    NOT NULL DEFAULT 0,
			available_amount REAL    NOT NULL DEFAULT 0,
			recorded_at      DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS kis_api_logs (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			endpoint     TEXT    NOT NULL,
			error_code   TEXT    NOT NULL DEFAULT '',
			error_message TEXT   NOT NULL DEFAULT '',
			raw_response TEXT    NOT NULL DEFAULT '',
			timestamp    DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS monitored_positions (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			stock_code   TEXT    NOT NULL UNIQUE,
			stock_name   TEXT    NOT NULL DEFAULT '',
			filled_price REAL    NOT NULL DEFAULT 0,
			target_price REAL    NOT NULL DEFAULT 0,
			stop_price   REAL    NOT NULL DEFAULT 0,
			order_id     INTEGER NOT NULL DEFAULT 0,
			created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS reports (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			report_date TEXT    NOT NULL UNIQUE,
			content     TEXT    NOT NULL DEFAULT '',
			created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec migration: %w\nSQL: %s", err, s)
		}
	}

	// 기존 DB 인스턴스를 위한 컬럼 추가 마이그레이션 (이미 존재하면 무시)
	alterStmts := []string{
		`ALTER TABLE orders ADD COLUMN stock_name   TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE orders ADD COLUMN filled_price REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE orders ADD COLUMN source       TEXT NOT NULL DEFAULT 'AGENT'`,
		`ALTER TABLE orders ADD COLUMN target_pct   REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE orders ADD COLUMN stop_pct     REAL NOT NULL DEFAULT 0`,
	}
	for _, s := range alterStmts {
		// "duplicate column name" 에러는 정상 (이미 존재하는 경우) — 무시
		db.Exec(s) //nolint:errcheck
	}

	// Default trading settings (INSERT OR IGNORE — never overwrite user-set values)
	defaultSettings := []struct{ key, val string }{
		{"take_profit_pct", "3.0"},
		{"stop_loss_pct", "2.0"},
		{"ranking_types", `["volume","strength","exec_count","disparity"]`},
		{"ranking_price_min", "5000"},
		{"ranking_price_max", "100000"},
		{"max_positions", "1"},
		{"order_amount_pct", "95"},
		{"sell_conditions", `["target_pct","stop_pct"]`},
		{"indicator_check_interval_min", "5"},
		{"indicator_rsi_sell_threshold", "70"},
		{"indicator_macd_bearish_sell", "false"},
		{"claude_model", "claude-sonnet-4-6"},
		{"ranking_volume_min_incrrate", "0"},
		{"ranking_strength_min", "100"},
		{"ranking_execcount_net_buy_only", "true"},
		{"ranking_disparity_d20_min", "0"},
		{"ranking_disparity_d20_max", "0"},
	}
	for _, s := range defaultSettings {
		db.Exec( //nolint:errcheck
			`INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))`,
			s.key, s.val)
	}

	return nil
}

// GetTradingSettings reads all autonomous trading settings from the DB in one call.
func (db *DB) GetTradingSettings(ctx context.Context) (TradingSettings, error) {
	keys := []string{
		"take_profit_pct", "stop_loss_pct", "ranking_types",
		"ranking_price_min", "ranking_price_max", "max_positions",
		"order_amount_pct", "sell_conditions", "indicator_check_interval_min",
		"indicator_rsi_sell_threshold", "indicator_macd_bearish_sell", "claude_model",
	}
	vals := make(map[string]string, len(keys))
	rows, err := db.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE key IN (`+
			`'take_profit_pct','stop_loss_pct','ranking_types',`+
			`'ranking_price_min','ranking_price_max','max_positions',`+
			`'order_amount_pct','sell_conditions','indicator_check_interval_min',`+
			`'indicator_rsi_sell_threshold','indicator_macd_bearish_sell','claude_model',`+
			`'ranking_volume_min_incrrate','ranking_strength_min',`+
			`'ranking_execcount_net_buy_only','ranking_disparity_d20_min','ranking_disparity_d20_max'`+
			`)`)
	if err != nil {
		return TradingSettings{}, fmt.Errorf("GetTradingSettings query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			vals[k] = v
		}
	}
	_ = keys

	// Parse helpers
	f64 := func(k string) float64 {
		v, _ := strconv.ParseFloat(vals[k], 64)
		return v
	}
	i64 := func(k string) int {
		v, _ := strconv.Atoi(vals[k])
		return v
	}
	strSlice := func(k string) []string {
		var s []string
		if v := vals[k]; v != "" {
			_ = json.Unmarshal([]byte(v), &s)
		}
		return s
	}

	takeProfitPct := f64("take_profit_pct")
	if takeProfitPct == 0 {
		takeProfitPct = 3.0
	}
	stopLossPct := f64("stop_loss_pct")
	if stopLossPct == 0 {
		stopLossPct = 2.0
	}
	maxPositions := i64("max_positions")
	if maxPositions == 0 {
		maxPositions = 1
	}
	orderAmountPct := f64("order_amount_pct")
	if orderAmountPct == 0 {
		orderAmountPct = 95
	}
	indicatorCheckInterval := i64("indicator_check_interval_min")
	if indicatorCheckInterval == 0 {
		indicatorCheckInterval = 5
	}
	rsiThreshold := f64("indicator_rsi_sell_threshold")
	if rsiThreshold == 0 {
		rsiThreshold = 70
	}
	claudeModel := vals["claude_model"]
	if claudeModel == "" {
		claudeModel = "claude-sonnet-4-6"
	}

	rankingTypes := strSlice("ranking_types")
	if len(rankingTypes) == 0 {
		rankingTypes = []string{"volume", "strength", "exec_count", "disparity"}
	}
	sellConditions := strSlice("sell_conditions")
	if len(sellConditions) == 0 {
		sellConditions = []string{"target_pct", "stop_pct"}
	}

	return TradingSettings{
		TakeProfitPct:              takeProfitPct,
		StopLossPct:                stopLossPct,
		RankingTypes:               rankingTypes,
		RankingPriceMin:            vals["ranking_price_min"],
		RankingPriceMax:            vals["ranking_price_max"],
		MaxPositions:               maxPositions,
		OrderAmountPct:             orderAmountPct,
		SellConditions:             sellConditions,
		IndicatorCheckIntervalMin:  indicatorCheckInterval,
		IndicatorRSISellThreshold:  rsiThreshold,
		IndicatorMACDBearishSell:   vals["indicator_macd_bearish_sell"] == "true",
		ClaudeModel:                claudeModel,
		RankingVolumeMinIncrRate:   f64("ranking_volume_min_incrrate"),
		RankingStrengthMin:         f64("ranking_strength_min"),
		RankingExecCountNetBuyOnly: vals["ranking_execcount_net_buy_only"] != "false",
		RankingDisparityD20Min:     f64("ranking_disparity_d20_min"),
		RankingDisparityD20Max:     f64("ranking_disparity_d20_max"),
	}, nil
}

// SaveReport upserts a daily trading report.
func (db *DB) SaveReport(ctx context.Context, reportDate, content string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO reports (report_date, content, created_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(report_date) DO UPDATE SET content = excluded.content, created_at = excluded.created_at`,
		reportDate, content)
	return err
}

// GetSetting returns the value for the given key from the settings table.
// Returns an empty string if the key does not exist.
func (db *DB) GetSetting(ctx context.Context, key string) string {
	var value string
	db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value) //nolint:errcheck
	return value
}

// SetSetting upserts a key-value pair in the settings table.
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value)
	return err
}
