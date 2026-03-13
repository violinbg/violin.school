package server

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/violinbg/violin.school/internal/ai"
)

func registerAIAdminRoutes(admin *gin.RouterGroup, db *sql.DB) {
	admin.GET("/admin/ai/settings", handleGetAISettings(db))
	admin.PATCH("/admin/ai/settings", handlePatchAISettings(db))
	admin.POST("/admin/ai/test", handleTestAISettings(db))
	admin.GET("/admin/ai/usage", handleGetAIUsage(db))
}

func aiService(db *sql.DB) (*ai.Service, error) {
	rawKey := strings.TrimSpace(os.Getenv("XAI_TOKEN_ENCRYPTION_KEY"))
	if rawKey == "" {
		return ai.NewService(db, nil), nil
	}

	key, err := ai.ParseEncryptionKey(rawKey)
	if err != nil {
		return nil, err
	}

	return ai.NewService(db, key), nil
}

func handleGetAISettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		service, err := aiService(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		settings, err := service.GetSettings(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		usage, err := service.GetUsageSummary(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"settings": settings,
			"usage":    usage,
		})
	}
}

func handlePatchAISettings(db *sql.DB) gin.HandlerFunc {
	type request struct {
		BaseURL                         *string                     `json:"base_url"`
		ApiToken                        *string                     `json:"api_token"`
		MonthlyGlobalLimitUsdTicks      *int64                      `json:"monthly_global_limit_usd_ticks"`
		MonthlyDefaultUserLimitUsdTicks *int64                      `json:"monthly_default_user_limit_usd_ticks"`
		UserLimitOverrides              []ai.UserLimitOverrideInput `json:"user_limit_overrides"`
	}

	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		service, err := aiService(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if err := service.UpdateSettings(c.Request.Context(), ai.UpdateSettingsInput{
			BaseURL:                         req.BaseURL,
			ApiToken:                        req.ApiToken,
			MonthlyGlobalLimitUsdTicks:      req.MonthlyGlobalLimitUsdTicks,
			MonthlyDefaultUserLimitUsdTicks: req.MonthlyDefaultUserLimitUsdTicks,
			UserLimitOverrides:              req.UserLimitOverrides,
			UpdatedBy:                       c.GetString(contextKeyUserID),
		}); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "ai settings updated"})
	}
}

func handleTestAISettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		service, err := aiService(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		metrics, err := service.TestConfiguredToken(c.Request.Context())
		if err != nil {
			statusCode := http.StatusBadRequest
			var budgetErr *ai.BudgetExceededError
			if errors.As(err, &budgetErr) {
				statusCode = http.StatusPaymentRequired
			}
			c.JSON(statusCode, gin.H{"error": err.Error()})
			return
		}

		if err := service.RecordUsage(c.Request.Context(), ai.RecordUsageInput{
			UserID:         c.GetString(contextKeyUserID),
			Provider:       "xai",
			Model:          metrics.Model,
			RequestScope:   "admin_test",
			TotalTokens:    metrics.TotalTokens,
			CostInUsdTicks: metrics.CostInUsdTicks,
			RequestID:      metrics.RequestID,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "xai token test succeeded"})
	}
}

func handleGetAIUsage(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		service, err := aiService(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		usage, err := service.GetUsageSummary(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, usage)
	}
}
