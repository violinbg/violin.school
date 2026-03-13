package server

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/violinbg/violin.school/internal/auth"
	"github.com/violinbg/violin.school/internal/models"
)

func registerUserRoutes(admin *gin.RouterGroup, db *sql.DB) {
	admin.GET("/users", handleListUsers(db))
	admin.POST("/users", handleCreateUser(db))
	admin.PUT("/users/:id", handleUpdateUser(db))
	admin.DELETE("/users/:id", handleDeleteUser(db))
	admin.PATCH("/users/:id/status", handleToggleUserStatus(db))
}

func handleListUsers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(
			"SELECT id, username, full_name, role, active, created_at, updated_at, last_login FROM users WHERE id != ? ORDER BY created_at DESC",
			systemAccountID,
		)
		if err != nil {
			log.Printf("handleListUsers: query error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		defer rows.Close()

		users := []gin.H{}
		for rows.Next() {
			var u models.User
			err := rows.Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt, &u.LastLogin)
			if err != nil {
				log.Printf("handleListUsers: scan error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
				return
			}
			users = append(users, gin.H{
				"id":         u.ID,
				"username":   u.Username,
				"full_name":  u.FullName,
				"role":       u.Role,
				"active":     u.Active,
				"created_at": u.CreatedAt,
				"updated_at": u.UpdatedAt,
				"last_login": u.LastLogin,
			})
		}
		if err := rows.Err(); err != nil {
			log.Printf("handleListUsers: rows error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"users": users})
	}
}

func handleCreateUser(db *sql.DB) gin.HandlerFunc {
	type request struct {
		Username string `json:"username" binding:"required,min=3"`
		FullName string `json:"full_name" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
		Role     string `json:"role" binding:"required,oneof=admin user"`
	}

	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Check if username already exists
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", req.Username).Scan(&exists)
		if err != nil {
			log.Printf("handleCreateUser: exists check error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
			return
		}

		hash, err := auth.Hash(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
			return
		}

		userID := uuid.New().String()
		now := time.Now().UTC()
		adminID := c.GetString(contextKeyUserID)
		if adminID == "" {
			adminID = systemAccountID
		}

		_, err = db.Exec(
			"INSERT INTO users (id, username, full_name, password_hash, role, active, created_by, created_at, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?)",
			userID, req.Username, req.FullName, hash, req.Role, adminID, now, adminID, now,
		)
		if err != nil {
			log.Printf("handleCreateUser: insert error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":         userID,
			"username":   req.Username,
			"full_name":  req.FullName,
			"role":       req.Role,
			"active":     true,
			"created_at": now,
			"updated_at": now,
			"last_login": nil,
		})
	}
}

func handleUpdateUser(db *sql.DB) gin.HandlerFunc {
	type request struct {
		FullName string `json:"full_name"`
		Role     string `json:"role" binding:"omitempty,oneof=admin user"`
		Password string `json:"password" binding:"omitempty,min=8"`
	}

	return func(c *gin.Context) {
		userID := c.Param("id")
		adminID := c.GetString(contextKeyUserID)
		if userID == systemAccountID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot modify system account"})
			return
		}

		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Prevent admin from changing their own role to non-admin
		if userID == adminID && req.Role != "" && req.Role != "admin" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot change your own role to non-admin"})
			return
		}

		// Build update query dynamically — updated_at is always refreshed
		updates := "full_name = ?, updated_by = ?, updated_at = CURRENT_TIMESTAMP"
		args := []interface{}{req.FullName, adminID, userID}

		if req.Role != "" {
			updates = "full_name = ?, role = ?, updated_by = ?, updated_at = CURRENT_TIMESTAMP"
			args = []interface{}{req.FullName, req.Role, adminID, userID}
		}

		if req.Password != "" {
			hash, err := auth.Hash(req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
				return
			}
			if req.Role != "" {
				updates = "full_name = ?, role = ?, password_hash = ?, updated_by = ?, updated_at = CURRENT_TIMESTAMP"
				args = []interface{}{req.FullName, req.Role, hash, adminID, userID}
			} else {
				updates = "full_name = ?, password_hash = ?, updated_by = ?, updated_at = CURRENT_TIMESTAMP"
				args = []interface{}{req.FullName, hash, adminID, userID}
			}
		}

		query := "UPDATE users SET " + updates + " WHERE id = ?"
		result, err := db.Exec(query, args...)
		if err != nil {
			log.Printf("handleUpdateUser: update error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		if n, _ := result.RowsAffected(); n == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		// Fetch updated user
		var u models.User
		err = db.QueryRow(
			"SELECT id, username, full_name, role, active, created_at, updated_at, last_login FROM users WHERE id = ?",
			userID,
		).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt, &u.LastLogin)
		if err != nil {
			log.Printf("handleUpdateUser: select error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id":         u.ID,
			"username":   u.Username,
			"full_name":  u.FullName,
			"role":       u.Role,
			"active":     u.Active,
			"created_at": u.CreatedAt,
			"updated_at": u.UpdatedAt,
			"last_login": u.LastLogin,
		})
	}
}

func handleDeleteUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")
		adminID := c.GetString(contextKeyUserID)
		if userID == systemAccountID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete system account"})
			return
		}

		// Prevent deletion of self
		if userID == adminID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
			return
		}

		tx, err := db.Begin()
		if err != nil {
			log.Printf("handleDeleteUser: begin tx error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		defer tx.Rollback() //nolint:errcheck

		// Delete all user data (CASCADE handled by foreign keys)
		result, err := tx.Exec("DELETE FROM users WHERE id = ?", userID)
		if err != nil {
			log.Printf("handleDeleteUser: delete error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		if n, _ := result.RowsAffected(); n == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("handleDeleteUser: commit error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
	}
}

func handleToggleUserStatus(db *sql.DB) gin.HandlerFunc {
	type request struct {
		Active *bool `json:"active" binding:"required"`
	}

	return func(c *gin.Context) {
		userID := c.Param("id")
		adminID := c.GetString(contextKeyUserID)
		if userID == systemAccountID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot change system account status"})
			return
		}

		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Active == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "active is required"})
			return
		}

		// Prevent deactivation of self
		if userID == adminID && !*req.Active {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot deactivate your own account"})
			return
		}

		result, err := db.Exec("UPDATE users SET active = ?, updated_by = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.Active, adminID, userID)
		if err != nil {
			log.Printf("handleToggleUserStatus: update error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		if n, _ := result.RowsAffected(); n == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		// Fetch updated user
		var u models.User
		err = db.QueryRow(
			"SELECT id, username, full_name, role, active, created_at, updated_at, last_login FROM users WHERE id = ?",
			userID,
		).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt, &u.LastLogin)
		if err != nil {
			log.Printf("handleToggleUserStatus: select error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id":         u.ID,
			"username":   u.Username,
			"full_name":  u.FullName,
			"role":       u.Role,
			"active":     u.Active,
			"created_at": u.CreatedAt,
			"updated_at": u.UpdatedAt,
			"last_login": u.LastLogin,
		})
	}
}
