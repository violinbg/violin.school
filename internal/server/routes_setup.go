package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/violinbg/violin.school/internal/auth"
)

func registerSetupRoutes(api *gin.RouterGroup, db *sql.DB) {
	api.GET("/setup/status", handleSetupStatus(db))
	api.POST("/setup", handleSetup(db))
}

// isInitialized reports whether a non-system user exists.
func isInitialized(db *sql.DB) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE id != ?", systemAccountID).Scan(&count)
	return count > 0, err
}

func handleSetupStatus(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ok, err := isInitialized(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var regEnabled string
		db.QueryRow("SELECT value FROM app_config WHERE key = 'registration_enabled'").Scan(&regEnabled) //nolint:errcheck

		c.JSON(http.StatusOK, gin.H{
			"initialized":          ok,
			"registration_enabled": regEnabled == "true",
		})
	}
}

func handleSetup(db *sql.DB) gin.HandlerFunc {
	type request struct {
		Username string `json:"username" binding:"required,min=3"`
		FullName string `json:"full_name" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}

	return func(c *gin.Context) {
		initialized, err := isInitialized(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if initialized {
			c.JSON(http.StatusConflict, gin.H{"error": "app already initialized"})
			return
		}

		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		hash, err := auth.Hash(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
			return
		}

		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate secret"})
			return
		}
		secret := hex.EncodeToString(secretBytes)

		userID := uuid.New().String()

		tx, err := db.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer tx.Rollback() //nolint:errcheck

		now := time.Now().UTC()
		if _, err := tx.Exec(
			"INSERT INTO users (id, username, full_name, password_hash, role, active, created_by, created_at, updated_by, updated_at) VALUES (?, ?, ?, ?, 'admin', 1, ?, ?, ?, ?)",
			userID, req.Username, req.FullName, hash, userID, now, userID, now,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if _, err := tx.Exec(
			"INSERT INTO app_config (key, value, created_by, created_at, updated_by, updated_at) VALUES ('jwt_secret', ?, ?, ?, ?, ?)",
			secret, userID, now, userID, now,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		for _, kv := range [][]string{
			{"registration_enabled", "true"},
			{"max_users", "100"},
		} {
			if _, err := tx.Exec(
				"INSERT OR IGNORE INTO app_config (key, value, created_by, created_at, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
				kv[0], kv[1], userID, now, userID, now,
			); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"message": "setup complete"})
	}
}
