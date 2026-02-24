package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// SetupRouter registers all API routes and returns a configured gin.Engine.
// frontendDist is the path to the React build output (dist/) directory.
func SetupRouter(h *Handler, frontendDist string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(jsonLogger())

	api := r.Group("/api")
	{
		api.GET("/balance", h.GetBalance)
		api.GET("/orders", h.GetOrders)
		api.POST("/orders", h.PlaceOrder)
		api.GET("/logs/kis", h.GetKISLogs)
		api.GET("/settings", h.GetSettings)
		api.GET("/debug/balance", h.DebugRawBalance)
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// SPA static file serving.
	// Requests not matched by /api/* or /health are served from frontendDist.
	// If the exact file exists, serve it; otherwise fall back to index.html
	// so that React Router can handle client-side navigation.
	r.NoRoute(func(c *gin.Context) {
		filePath := filepath.Join(frontendDist, filepath.Clean(c.Request.URL.Path))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			c.File(filePath)
			return
		}
		index := filepath.Join(frontendDist, "index.html")
		if _, err := os.Stat(index); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "frontend not found"})
			return
		}
		c.File(index)
	})

	return r
}

// jsonLogger is a minimal structured request logger middleware.
func jsonLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Output is handled by the logger package; keep gin's default quiet.
		return ""
	})
}
