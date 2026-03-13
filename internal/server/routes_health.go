package server

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func registerHealthRoutes(api *gin.RouterGroup, db *sql.DB) {
	api.GET("/health", handleHealth(db))
}

func handleHealth(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := db.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "detail": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
