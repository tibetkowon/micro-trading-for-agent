package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

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
			is_mock      INTEGER NOT NULL DEFAULT 1,
			issued_at    DATETIME NOT NULL DEFAULT (datetime('now')),
			expires_at   DATETIME NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS orders (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			stock_code   TEXT    NOT NULL,
			order_type   TEXT    NOT NULL CHECK(order_type IN ('BUY','SELL')),
			qty          INTEGER NOT NULL CHECK(qty > 0),
			price        REAL    NOT NULL CHECK(price >= 0),
			status       TEXT    NOT NULL DEFAULT 'PENDING'
			                CHECK(status IN ('PENDING','FILLED','CANCELLED','FAILED')),
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
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec migration: %w\nSQL: %s", err, s)
		}
	}

	// Add is_mock column to existing tokens table if it doesn't exist yet.
	_, _ = db.Exec(`ALTER TABLE tokens ADD COLUMN is_mock INTEGER NOT NULL DEFAULT 1`)

	return nil
}
