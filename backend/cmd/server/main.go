package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // NCP Micro 이미지에 tzdata 없을 경우 Asia/Seoul 로드 실패 방지
	"time"

	"github.com/micro-trading-for-agent/backend/internal/agent"
	"github.com/micro-trading-for-agent/backend/internal/api"
	"github.com/micro-trading-for-agent/backend/internal/config"
	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("database initialized", map[string]any{"path": cfg.DatabasePath})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize KIS token manager.
	tokenManager := kis.NewTokenManager(cfg.KISBaseURL, cfg.KISAppKey, cfg.KISAppSecret, db)
	if cfg.KISAppKey != "" && cfg.KISAppSecret != "" {
		// Invalidate cached tokens if credentials have changed since last run.
		if err := tokenManager.InvalidateIfCredentialsChanged(ctx); err != nil {
			logger.Warn("credential check failed — token cache may be stale",
				map[string]any{"error": err.Error()})
		}
		if _, err := tokenManager.EnsureToken(ctx); err != nil {
			logger.Warn("initial KIS token issue failed — API calls will fail until resolved",
				map[string]any{"error": err.Error()})
		}
		tokenManager.StartAutoRefresh(ctx)
		defer tokenManager.Stop()
	} else {
		logger.Warn("KIS_APP_KEY or KIS_APP_SECRET not set — running without KIS integration", nil)
	}

	kisClient := kis.NewClient(
		cfg.KISBaseURL,
		cfg.KISAppKey,
		cfg.KISAppSecret,
		cfg.KISAccountNo,
		cfg.KISAccountType,
		tokenManager,
		db,
	)

	if cfg.KISAppKey != "" && cfg.KISAppSecret != "" {
		agent.StartOrderSyncScheduler(ctx, kisClient, db, 3*time.Minute)
		logger.Info("order sync scheduler started", map[string]any{"interval": "3m"})
	}

	handler := api.NewHandler(db, kisClient, tokenManager, cfg)
	router := api.SetupRouter(handler, cfg.FrontendDist)

	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("server starting", map[string]any{"port": cfg.ServerPort})
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-quit
	logger.Info("shutting down server...", nil)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("forced shutdown", map[string]any{"error": err.Error()})
	}
	logger.Info("server exited", nil)
}
