package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/violinbg/violin.school/internal/auth"
	"github.com/violinbg/violin.school/internal/captcha"
	"github.com/violinbg/violin.school/internal/models"
)

const refreshTokenTTL = 7 * 24 * time.Hour

// cleanupMu guards lastCleanup so expired-token purges run at most once per minute.
var (
	cleanupMu   sync.Mutex
	lastCleanup time.Time
)

// hashToken returns the lowercase hex SHA-256 of the raw token string.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// cleanupExpiredTokens deletes expired rows from refresh_tokens, throttled to
// at most once per minute. It is safe to call from a goroutine.
func cleanupExpiredTokens(db *sql.DB) {
	cleanupMu.Lock()
	if time.Since(lastCleanup) < time.Minute {
		cleanupMu.Unlock()
		return
	}
	lastCleanup = time.Now()
	cleanupMu.Unlock()

	db.Exec("DELETE FROM refresh_tokens WHERE expires_at < ?", time.Now().UTC()) //nolint:errcheck
}

func registerAuthRoutes(api *gin.RouterGroup, protected *gin.RouterGroup, db *sql.DB, cs *captcha.Store) {
	api.POST("/auth/login", handleLogin(db))
	api.GET("/auth/captcha", handleGetCaptcha(cs))
	api.POST("/auth/register", handleRegister(db, cs))
	api.POST("/auth/refresh", handleRefresh(db))
	api.POST("/auth/logout", handleLogout(db))
	protected.GET("/auth/me", handleMe(db))
	protected.PATCH("/profile/language", handleUpdateLanguage(db))
}

// jwtSecret fetches the jwt_secret from app_config, returning "" on any error.
func jwtSecret(db *sql.DB) string {
	var secret string
	db.QueryRow("SELECT value FROM app_config WHERE key = 'jwt_secret'").Scan(&secret) //nolint:errcheck
	return secret
}

// issueTokenPair creates a signed access JWT and a new refresh token stored in the DB.
// family identifies the rotation chain; pass "" to start a new family (login / register).
// The raw refresh token is returned to the caller; only its SHA-256 hash is persisted.
// It returns (accessToken, rawRefreshToken, error).
func issueTokenPair(db *sql.DB, userID, username, fullName, role, family string) (string, string, error) {
	secret := jwtSecret(db)
	accessToken, err := auth.Sign(userID, username, fullName, role, secret)
	if err != nil {
		return "", "", err
	}

	if family == "" {
		family = uuid.New().String()
	}

	rawRefresh := uuid.New().String()
	now := time.Now().UTC()
	expiresAt := now.Add(refreshTokenTTL)
	if _, err := db.Exec(
		"INSERT INTO refresh_tokens (token_hash, family, user_id, expires_at, used, created_by, created_at, updated_by, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?)",
		hashToken(rawRefresh), family, userID, expiresAt, userID, now, userID, now,
	); err != nil {
		return "", "", err
	}

	return accessToken, rawRefresh, nil
}

// issueTokenPairTx is like issueTokenPair but inserts the new refresh token
// within an existing transaction tx. The JWT secret is read via db (outside the tx).
func issueTokenPairTx(tx *sql.Tx, db *sql.DB, userID, username, fullName, role, family string) (string, string, error) {
	secret := jwtSecret(db)
	accessToken, err := auth.Sign(userID, username, fullName, role, secret)
	if err != nil {
		return "", "", err
	}

	if family == "" {
		family = uuid.New().String()
	}

	rawRefresh := uuid.New().String()
	now := time.Now().UTC()
	expiresAt := now.Add(refreshTokenTTL)
	if _, err := tx.Exec(
		"INSERT INTO refresh_tokens (token_hash, family, user_id, expires_at, used, created_by, created_at, updated_by, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?)",
		hashToken(rawRefresh), family, userID, expiresAt, userID, now, userID, now,
	); err != nil {
		return "", "", err
	}

	return accessToken, rawRefresh, nil
}

func handleLogin(db *sql.DB) gin.HandlerFunc {
	type request struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var u models.User
		err := db.QueryRow(
			"SELECT id, username, full_name, password_hash, role, active, language FROM users WHERE username = ?",
			req.Username,
		).Scan(&u.ID, &u.Username, &u.FullName, &u.PasswordHash, &u.Role, &u.Active, &u.Language)
		if err == sql.ErrNoRows || !auth.Check(u.PasswordHash, req.Password) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		if !u.Active {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		now := time.Now().UTC()
		_, err = db.Exec("UPDATE users SET last_login = ?, updated_by = ?, updated_at = ? WHERE id = ?", now, u.ID, now, u.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update login time"})
			return
		}

		accessToken, refreshToken, err := issueTokenPair(db, u.ID, u.Username, u.FullName, u.Role, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue tokens"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"user": gin.H{
				"id":        u.ID,
				"username":  u.Username,
				"full_name": u.FullName,
				"role":      u.Role,
				"language":  u.Language,
			},
		})
	}
}

func handleMe(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(contextKeyUserID)
		var language string
		if err := db.QueryRow("SELECT language FROM users WHERE id = ?", userID).Scan(&language); err != nil {
			language = "en"
		}
		c.JSON(http.StatusOK, gin.H{
			"id":        userID,
			"username":  c.GetString(contextKeyUsername),
			"full_name": c.GetString(contextKeyFullName),
			"role":      c.GetString(contextKeyRole),
			"language":  language,
		})
	}
}

func handleRefresh(db *sql.DB) gin.HandlerFunc {
	type request struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	return func(c *gin.Context) {
		// Opportunistic cleanup of expired tokens (throttled to once per minute).
		go cleanupExpiredTokens(db)

		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		incomingHash := hashToken(req.RefreshToken)

		tx, err := db.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		defer tx.Rollback() //nolint:errcheck

		var family, userID string
		var expiresAt time.Time
		var used int
		err = tx.QueryRow(
			"SELECT family, user_id, expires_at, used FROM refresh_tokens WHERE token_hash = ?",
			incomingHash,
		).Scan(&family, &userID, &expiresAt, &used)
		if err == sql.ErrNoRows {
			// Token not found — either never existed or already consumed.
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		// Reuse detected: a token that was already rotated is being replayed.
		// Revoke the entire family to protect the legitimate session.
		if used == 1 {
			tx.Exec("DELETE FROM refresh_tokens WHERE family = ?", family) //nolint:errcheck
			tx.Commit()                                                    //nolint:errcheck
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
			return
		}

		if time.Now().UTC().After(expiresAt) {
			tx.Exec("DELETE FROM refresh_tokens WHERE token_hash = ?", incomingHash) //nolint:errcheck
			tx.Commit()                                                              //nolint:errcheck
			c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token expired"})
			return
		}

		// Fetch current user data and verify the account is still active.
		var u models.User
		err = tx.QueryRow(
			"SELECT id, username, full_name, role, active FROM users WHERE id = ?", userID,
		).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.Active)
		if err != nil || !u.Active {
			tx.Exec("DELETE FROM refresh_tokens WHERE token_hash = ?", incomingHash) //nolint:errcheck
			tx.Commit()                                                              //nolint:errcheck
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
			return
		}

		// Mark old token as used (kept for replay detection until it expires).
		if _, err := tx.Exec("UPDATE refresh_tokens SET used = 1 WHERE token_hash = ?", incomingHash); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		// Issue new pair in the same family, within the transaction.
		accessToken, newRawRefresh, err := issueTokenPairTx(tx, db, u.ID, u.Username, u.FullName, u.Role, family)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue tokens"})
			return
		}

		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"access_token":  accessToken,
			"refresh_token": newRawRefresh,
		})
	}
}

func handleLogout(db *sql.DB) gin.HandlerFunc {
	type request struct {
		RefreshToken string `json:"refresh_token"`
	}

	return func(c *gin.Context) {
		var req request
		c.ShouldBindJSON(&req) //nolint:errcheck
		if req.RefreshToken != "" {
			db.Exec("DELETE FROM refresh_tokens WHERE token_hash = ?", hashToken(req.RefreshToken)) //nolint:errcheck
		}
		c.Status(http.StatusNoContent)
	}
}

func handleGetCaptcha(cs *captcha.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, question := cs.Generate()
		c.JSON(http.StatusOK, gin.H{
			"id":       id,
			"question": question,
		})
	}
}

func handleRegister(db *sql.DB, cs *captcha.Store) gin.HandlerFunc {
	type request struct {
		Username      string `json:"username" binding:"required,min=3"`
		FullName      string `json:"full_name" binding:"required"`
		Password      string `json:"password" binding:"required,min=8"`
		CaptchaID     string `json:"captcha_id" binding:"required"`
		CaptchaAnswer string `json:"captcha_answer" binding:"required"`
	}

	return func(c *gin.Context) {
		var enabled string
		db.QueryRow("SELECT value FROM app_config WHERE key = 'registration_enabled'").Scan(&enabled) //nolint:errcheck
		if enabled != "true" {
			c.JSON(http.StatusForbidden, gin.H{"error": "registration is currently disabled"})
			return
		}

		var maxUsersStr string
		db.QueryRow("SELECT value FROM app_config WHERE key = 'max_users'").Scan(&maxUsersStr) //nolint:errcheck
		maxUsers, _ := strconv.Atoi(maxUsersStr)
		if maxUsers <= 0 {
			maxUsers = 100
		}
		var userCount int
		db.QueryRow("SELECT COUNT(*) FROM users WHERE id != ?", systemAccountID).Scan(&userCount) //nolint:errcheck
		if userCount >= maxUsers {
			c.JSON(http.StatusForbidden, gin.H{"error": "maximum number of users reached"})
			return
		}

		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if !cs.Verify(req.CaptchaID, req.CaptchaAnswer) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "incorrect captcha answer"})
			return
		}

		var exists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", req.Username).Scan(&exists) //nolint:errcheck
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
			return
		}

		hash, err := auth.Hash(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
			return
		}

		userID := uuid.New().String()
		now := time.Now().UTC()
		if _, err := db.Exec(
			"INSERT INTO users (id, username, full_name, password_hash, role, active, created_by, created_at, updated_by, updated_at) VALUES (?, ?, ?, ?, 'user', 1, ?, ?, ?, ?)",
			userID, req.Username, req.FullName, hash, userID, now, userID, now,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create user"})
			return
		}

		accessToken, refreshToken, err := issueTokenPair(db, userID, req.Username, req.FullName, "user", "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue tokens"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"user": gin.H{
				"id":        userID,
				"username":  req.Username,
				"full_name": req.FullName,
				"role":      "user",
				"language":  "en",
			},
		})
	}
}

func handleUpdateLanguage(db *sql.DB) gin.HandlerFunc {
	type request struct {
		Language string `json:"language" binding:"required"`
	}

	supported := map[string]bool{"en": true, "bg": true, "es": true, "ja": true, "ko": true, "zh": true}

	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if !supported[req.Language] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported language"})
			return
		}

		userID := c.GetString(contextKeyUserID)
		if _, err := db.Exec("UPDATE users SET language = ?, updated_by = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", req.Language, userID, userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		c.Status(http.StatusNoContent)
	}
}
