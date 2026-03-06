package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // NCP Micro 이미지에 tzdata 없을 경우 Asia/Seoul 로드 실패 방지

	"github.com/micro-trading-for-agent/backend/internal/agent"
	"github.com/micro-trading-for-agent/backend/internal/api"
	"github.com/micro-trading-for-agent/backend/internal/config"
	"github.com/micro-trading-for-agent/backend/internal/database"
	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/logger"
	"github.com/micro-trading-for-agent/backend/internal/monitor"
	mqttpkg "github.com/micro-trading-for-agent/backend/internal/mqtt"
	"github.com/micro-trading-for-agent/backend/internal/trader"
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
		if err := tokenManager.InvalidateIfCredentialsChanged(ctx); err != nil {
			logger.Warn("credential check failed — token cache may be stale",
				map[string]any{"error": err.Error()})
		}
		if _, err := tokenManager.EnsureToken(ctx); err != nil {
			logger.Warn("initial KIS token issue failed — API calls will fail until resolved",
				map[string]any{"error": err.Error()})
		}
		// 토큰 갱신은 매일 08:50 market scheduler에서 WebSocket 연결과 함께 수행.
		// 별도 20시간 타이머 없음.
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

	// --- MQTT Publisher (optional) ---
	var mqttPub *mqttpkg.Publisher
	if cfg.MQTTBrokerURL != "" {
		pub, mqttErr := mqttpkg.NewPublisher(cfg.MQTTBrokerURL, cfg.MQTTClientID)
		if mqttErr != nil {
			logger.Warn("MQTT broker unavailable — alerts will be logged only",
				map[string]any{"broker": cfg.MQTTBrokerURL, "error": mqttErr.Error()})
		} else {
			mqttPub = pub
			defer mqttPub.Close()
			logger.Info("MQTT publisher ready", map[string]any{"broker": cfg.MQTTBrokerURL})
		}
	}

	// --- KIS WebSocket client (optional — requires credentials) ---
	var wsClient *kis.WebSocketClient
	if cfg.KISAppKey != "" && cfg.KISAppSecret != "" {
		wsClient = kis.NewWebSocketClient("", cfg.KISHTSID)
		// approval_key is fetched just before market open; start with empty key.
	}

	// --- Position monitor ---
	mon := monitor.New(db, kisClient, wsClient, mqttPub)
	if err := mon.LoadFromDB(ctx); err != nil {
		logger.Warn("failed to restore monitored positions from DB",
			map[string]any{"error": err.Error()})
	}

	// --- Order sync scheduler (폴링 폴백) ---
	if cfg.KISAppKey != "" && cfg.KISAppSecret != "" {
		agent.StartOrderSyncScheduler(ctx, kisClient, db, 5*time.Minute)
		logger.Info("order sync scheduler started", map[string]any{"interval": "5m"})
	}

	// --- Trading engine (Claude-based autonomous trader) ---
	var claudeClient *trader.ClaudeClient
	if cfg.AnthropicAPIKey != "" {
		settings, _ := db.GetTradingSettings(ctx)
		claudeClient = trader.NewClaudeClient(cfg.AnthropicAPIKey, settings.ClaudeModel)
		logger.Info("Claude client initialized", map[string]any{"model": settings.ClaudeModel})
	} else {
		logger.Warn("ANTHROPIC_API_KEY not set — autonomous trading disabled", nil)
	}

	tradingEngine := trader.NewEngine(db, kisClient, wsClient, mon, mqttPub, claudeClient)

	// --- Market hours scheduler ---
	if cfg.KISAppKey != "" && cfg.KISAppSecret != "" && wsClient != nil {
		go runMarketScheduler(ctx, db, kisClient, wsClient, mon, tokenManager, tradingEngine)
	}

	// --- Price consumer ---
	if wsClient != nil {
		go mon.StartPriceConsumer(ctx)
	}

	handler := api.NewHandler(db, kisClient, tokenManager, cfg, mon, wsClient)
	handler.SetEngine(tradingEngine)
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

	if wsClient != nil {
		wsClient.Disconnect()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("forced shutdown", map[string]any{"error": err.Error()})
	}
	logger.Info("server exited", nil)
}

// runMarketScheduler manages WebSocket lifecycle and trading engine based on KST market hours:
//
//	08:50 → issue fresh token → fetch approval_key → connect → subscribe
//	09:00 → check trading_enabled + market open → set tradingReady
//	09:15 → start trading engine + indicator checker (if tradingReady)
//	15:15 → stop engine → liquidate all positions
//	15:20 → generate daily report → save to DB
//	16:00 → disconnect
func runMarketScheduler(ctx context.Context,
	db *database.DB, kisClient *kis.Client, wsClient *kis.WebSocketClient, mon *monitor.Monitor,
	tokenManager *kis.TokenManager, eng *trader.Engine) {

	kst, _ := time.LoadLocation("Asia/Seoul")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var wsRunning bool
	var engineRunning bool
	var tradingReady bool
	var stopEngine func()
	var stopIndicator context.CancelFunc

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().In(kst)
			wd := now.Weekday()
			if wd == time.Saturday || wd == time.Sunday {
				continue
			}
			hhmm := now.Hour()*100 + now.Minute()

			switch {
			case hhmm == 850 && !wsRunning:
				// 08:50 — issue fresh token + connect WebSocket
				if _, err := tokenManager.IssueToken(ctx); err != nil {
					logger.Error("08:50 token refresh failed", map[string]any{"error": err.Error()})
				} else {
					logger.Info("market scheduler: KIS token refreshed at 08:50", nil)
				}
				approvalKey, err := kisClient.GetApprovalKey(ctx)
				if err != nil {
					logger.Error("GetApprovalKey failed", map[string]any{"error": err.Error()})
					continue
				}

				wsClient.SetApprovalKey(approvalKey)
				go wsClient.StartWithReconnect(ctx)
				wsRunning = true
				tradingReady = false

				// Subscribe execution notices if HTS ID is configured.
				time.Sleep(2 * time.Second) // Wait for connection
				if err := wsClient.SubscribeExecNotice(); err != nil {
					logger.Warn("exec notice subscribe failed", map[string]any{"error": err.Error()})
				}
				logger.Info("market scheduler: WebSocket connected at 08:50", nil)

			case hhmm == 900 && !tradingReady:
				// 09:00 — verify trading is enabled and market is open
				tradingEnabled := db.GetSetting(ctx, "trading_enabled") != "false"
				if !tradingEnabled {
					logger.Info("market scheduler: trading disabled — engine will not start", nil)
					break
				}
				isOpen, err := agent.IsMarketOpen(ctx, kisClient)
				if err != nil || !isOpen {
					logger.Info("market scheduler: market not open at 09:00 — engine will not start",
						map[string]any{"is_open": isOpen})
					break
				}
				tradingReady = true
				logger.Info("market scheduler: trading ready confirmed at 09:00", nil)

			case hhmm == 915 && tradingReady && !engineRunning:
				// 09:15 — start autonomous trading engine
				settings, err := db.GetTradingSettings(ctx)
				if err != nil {
					logger.Error("market scheduler: GetTradingSettings failed", map[string]any{"error": err.Error()})
					break
				}

				stopEngine = eng.Start(ctx)
				engineRunning = true
				logger.Info("market scheduler: trading engine started at 09:15", nil)

				// Start indicator checker
				indCtx, indCancel := context.WithCancel(ctx)
				stopIndicator = indCancel
				go mon.StartIndicatorChecker(
					indCtx,
					settings.IndicatorCheckIntervalMin,
					settings.SellConditions,
					settings.IndicatorRSISellThreshold,
					settings.IndicatorMACDBearishSell,
					func(iCtx context.Context, code string) (*monitor.IndicatorSnapshot, error) {
						info, err := agent.GetStockInfo(iCtx, kisClient, code)
						if err != nil {
							return nil, err
						}
						return &monitor.IndicatorSnapshot{
							RSI14:      info.RSI14,
							MACDLine:   info.MACDLine,
							MACDSignal: info.MACDSignal,
						}, nil
					},
				)
				logger.Info("market scheduler: indicator checker started", nil)

			case hhmm == 1515:
				// 15:15 — stop engine, stop indicator checker, liquidate all
				if engineRunning && stopEngine != nil {
					stopEngine()
					engineRunning = false
				}
				if stopIndicator != nil {
					stopIndicator()
					stopIndicator = nil
				}
				logger.Info("market scheduler: 15:15 liquidation triggered", nil)
				mon.LiquidateAll(ctx)

			case hhmm == 1520:
				// 15:20 — generate daily report
				report, err := eng.GenerateDailyReport(ctx)
				if err != nil {
					logger.Error("market scheduler: daily report generation failed",
						map[string]any{"error": err.Error()})
				} else {
					kstDate := now.Format("2006-01-02")
					if saveErr := db.SaveReport(ctx, kstDate, report); saveErr != nil {
						logger.Error("market scheduler: save report failed",
							map[string]any{"error": saveErr.Error()})
					} else {
						logger.Info("market scheduler: daily report saved", map[string]any{"date": kstDate})
					}
				}

			case hhmm == 1600 && wsRunning:
				// 16:00 — disconnect
				wsClient.Disconnect()
				wsRunning = false
				tradingReady = false
				engineRunning = false
				logger.Info("market scheduler: WebSocket disconnected at 16:00", nil)
			}
		}
	}
}
