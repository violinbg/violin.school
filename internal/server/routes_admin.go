package server

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func registerAdminRoutes(admin *gin.RouterGroup, db *sql.DB) {
	admin.GET("/admin/settings", handleGetAdminSettings(db))
	admin.PATCH("/admin/settings", handlePatchAdminSettings(db))
}

func handleGetAdminSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings := map[string]string{}
		rows, err := db.Query("SELECT key, value FROM app_config WHERE key IN ('registration_enabled', 'max_users')")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			settings[k] = v
		}

		var userCount int
		db.QueryRow("SELECT COUNT(*) FROM users WHERE id != ?", systemAccountID).Scan(&userCount) //nolint:errcheck

		maxUsers, _ := strconv.Atoi(settings["max_users"])
		if maxUsers <= 0 {
			maxUsers = 100
		}

		c.JSON(http.StatusOK, gin.H{
			"registration_enabled": settings["registration_enabled"] == "true",
			"max_users":            maxUsers,
			"user_count":           userCount,
		})
	}
}

func handlePatchAdminSettings(db *sql.DB) gin.HandlerFunc {
	type request struct {
		RegistrationEnabled *bool `json:"registration_enabled"`
		MaxUsers            *int  `json:"max_users"`
	}

	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		now := time.Now().UTC()
		updatedBy := c.GetString(contextKeyUserID)
		if updatedBy == "" {
			updatedBy = systemAccountID
		}

		if req.RegistrationEnabled != nil {
			val := "false"
			if *req.RegistrationEnabled {
				val = "true"
			}
			if _, err := db.Exec(
				"INSERT INTO app_config (key, value, updated_by, updated_at) VALUES ('registration_enabled', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at",
				val, updatedBy, now,
			); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		if req.MaxUsers != nil {
			if *req.MaxUsers < 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "max_users must be at least 1"})
				return
			}
			if _, err := db.Exec(
				"INSERT INTO app_config (key, value, updated_by, updated_at) VALUES ('max_users', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at",
				strconv.Itoa(*req.MaxUsers), updatedBy, now,
			); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "settings updated"})
	}
}
