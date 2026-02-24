package kis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/logger"
	"github.com/micro-trading-for-agent/backend/internal/models"
)

const (
	// tokenRefreshInterval is 20 hours — safely before the 24-hour KIS expiry.
	tokenRefreshInterval = 20 * time.Hour
	tokenEndpoint        = "/oauth2/tokenP"
)

// TokenManager handles KIS access token lifecycle.
type TokenManager struct {
	mu        sync.RWMutex
	baseURL   string
	appKey    string
	appSecret string
	db        *database.DB
	stopCh    chan struct{}
}

// NewTokenManager creates a new TokenManager.
func NewTokenManager(baseURL string, appKey, appSecret string, db *database.DB) *TokenManager {
	return &TokenManager{
		baseURL:   baseURL,
		appKey:    appKey,
		appSecret: appSecret,
		db:        db,
		stopCh:    make(chan struct{}),
	}
}

// InvalidateIfCredentialsChanged clears the token cache when APP KEY or SECRET changes.
// It computes a SHA-256 fingerprint of the current credentials and compares it with
// the value stored in the settings table. If they differ, all tokens are deleted so
// EnsureToken will issue a fresh one.
func (tm *TokenManager) InvalidateIfCredentialsChanged(ctx context.Context) error {
	h := sha256.Sum256([]byte(tm.appKey + ":" + tm.appSecret))
	currentHash := fmt.Sprintf("%x", h)

	var storedHash string
	err := tm.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'kis_credentials_hash'`).Scan(&storedHash)

	if err == nil && storedHash == currentHash {
		// Credentials unchanged; existing tokens are valid.
		return nil
	}

	// Credentials changed or not yet stored — clear all tokens.
	if _, err := tm.db.ExecContext(ctx, `DELETE FROM tokens`); err != nil {
		return fmt.Errorf("clear tokens: %w", err)
	}

	_, err = tm.db.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ('kis_credentials_hash', ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		currentHash)
	if err != nil {
		return fmt.Errorf("update credentials hash: %w", err)
	}

	logger.Info("KIS credentials changed — token cache cleared", nil)
	return nil
}

// issueTokenResponse is the KIS token API response schema.
type issueTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // seconds
	MsgCode     string `json:"msg_cd"`
	Msg         string `json:"msg1"`
}

// IssueToken calls the KIS OAuth endpoint and persists the token to DB.
func (tm *TokenManager) IssueToken(ctx context.Context) (*models.Token, error) {
	tm.mu.RLock()
	baseURL := tm.baseURL
	tm.mu.RUnlock()

	payload := map[string]string{
		"grant_type": "client_credentials",
		"appkey":     tm.appKey,
		"appsecret":  tm.appSecret,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+tokenEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		logger.KISError(tokenEndpoint, fmt.Sprintf("HTTP-%d", resp.StatusCode), string(raw))
		return nil, fmt.Errorf("KIS token API returned %d", resp.StatusCode)
	}

	var res issueTokenResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		logger.KISError(tokenEndpoint, "PARSE_ERROR", string(raw))
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if res.AccessToken == "" {
		logger.KISError(tokenEndpoint, res.MsgCode, string(raw))
		return nil, fmt.Errorf("empty access token: %s", res.Msg)
	}

	now := time.Now()
	tok := &models.Token{
		AccessToken: res.AccessToken,
		IssuedAt:    now,
		ExpiresAt:   now.Add(time.Duration(res.ExpiresIn) * time.Second),
	}

	if err := tm.saveToken(tok); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}

	logger.Info("KIS access token issued", map[string]any{"expires_at": tok.ExpiresAt})
	return tok, nil
}

// GetCurrentToken returns the most recent valid token.
func (tm *TokenManager) GetCurrentToken(ctx context.Context) (*models.Token, error) {
	row := tm.db.QueryRowContext(ctx,
		`SELECT id, access_token, issued_at, expires_at
		 FROM tokens ORDER BY id DESC LIMIT 1`)

	var tok models.Token
	err := row.Scan(&tok.ID, &tok.AccessToken, &tok.IssuedAt, &tok.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("no token found: %w", err)
	}
	return &tok, nil
}

// EnsureToken reuses a valid token if it has more than 1 hour remaining,
// otherwise issues a new one.
// Prevents hitting KIS rate limits (1 issue/min) on repeated restarts.
func (tm *TokenManager) EnsureToken(ctx context.Context) (*models.Token, error) {
	tok, err := tm.GetCurrentToken(ctx)
	if err == nil && time.Until(tok.ExpiresAt) > time.Hour {
		logger.Info("reusing existing KIS token from DB",
			map[string]any{"expires_at": tok.ExpiresAt})
		return tok, nil
	}
	return tm.IssueToken(ctx)
}

// StartAutoRefresh launches a background goroutine that refreshes the token
// every tokenRefreshInterval (20 hours). Call Stop() to shut it down.
func (tm *TokenManager) StartAutoRefresh(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(tokenRefreshInterval)
		defer ticker.Stop()
		logger.Info("KIS token auto-refresh started", map[string]any{"interval": tokenRefreshInterval.String()})
		for {
			select {
			case <-ticker.C:
				if _, err := tm.IssueToken(ctx); err != nil {
					logger.Error("KIS token auto-refresh failed", map[string]any{"error": err.Error()})
				}
			case <-tm.stopCh:
				logger.Info("KIS token auto-refresh stopped", nil)
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop signals the auto-refresh goroutine to exit.
func (tm *TokenManager) Stop() {
	close(tm.stopCh)
}

func (tm *TokenManager) saveToken(tok *models.Token) error {
	_, err := tm.db.Exec(
		`INSERT INTO tokens (access_token, issued_at, expires_at) VALUES (?, ?, ?)`,
		tok.AccessToken, tok.IssuedAt, tok.ExpiresAt,
	)
	return err
}
