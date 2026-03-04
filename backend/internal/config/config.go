package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	KISAppKey      string
	KISAppSecret   string
	KISAccountNo   string
	KISAccountType string
	KISBaseURL     string
	KISHTSID       string // HTS ID — used as tr_key for 체결통보 (H0STCNI0)

	DatabasePath string
	ServerPort   string
	FrontendDist string

	MQTTBrokerURL string
	MQTTClientID  string
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
		KISHTSID:       getEnv("KIS_HTS_ID", ""),
		DatabasePath:   getEnv("DATABASE_PATH", "./data/trading.db"),
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		FrontendDist:   getEnv("FRONTEND_DIST_PATH", "./frontend/dist"),
		MQTTBrokerURL:  getEnv("MQTT_BROKER_URL", "tcp://localhost:1883"),
		MQTTClientID:   getEnv("MQTT_CLIENT_ID", "micro-trading-server"),
	}

	return cfg, nil
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
