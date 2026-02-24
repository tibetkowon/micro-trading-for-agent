package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	KISAppKey      string
	KISAppSecret   string
	KISAccountNo   string
	KISAccountType string
	KISBaseURL     string
	KISMockURL     string
	KISIsMock      bool

	DatabasePath string
	ServerPort   string
	FrontendDist string
}

// Load reads .env file (if present) and returns a populated Config.
// Actual secrets must never be hardcoded; they come from the environment only.
func Load() (*Config, error) {
	// Load .env file; ignore error if it does not exist (e.g., in production via env vars)
	_ = godotenv.Load()

	cfg := &Config{
		KISAppKey:      mustEnv("KIS_APP_KEY"),
		KISAppSecret:   mustEnv("KIS_APP_SECRET"),
		KISAccountNo:   mustEnv("KIS_ACCOUNT_NO"),
		KISAccountType: getEnv("KIS_ACCOUNT_TYPE", "01"),
		KISBaseURL:     getEnv("KIS_BASE_URL", "https://openapi.koreainvestment.com:9443"),
		KISMockURL:     getEnv("KIS_MOCK_URL", "https://openapivts.koreainvestment.com:29443"),
		DatabasePath:   getEnv("DATABASE_PATH", "./data/trading.db"),
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		FrontendDist:   getEnv("FRONTEND_DIST_PATH", "./frontend/dist"),
	}

	isMock, err := strconv.ParseBool(getEnv("KIS_IS_MOCK", "true"))
	if err != nil {
		return nil, fmt.Errorf("invalid KIS_IS_MOCK value: %w", err)
	}
	cfg.KISIsMock = isMock

	return cfg, nil
}

// BaseURL returns the active KIS API base URL depending on the mock flag.
func (c *Config) BaseURL() string {
	if c.KISIsMock {
		return c.KISMockURL
	}
	return c.KISBaseURL
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		// Return empty string; caller decides if this is fatal at startup.
		return ""
	}
	return v
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
