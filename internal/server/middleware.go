package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/violinbg/violin.school/internal/auth"
)

const contextKeyUserID = "userID"
const contextKeyUsername = "username"
const contextKeyFullName = "fullName"
const contextKeyRole = "role"

// AuthRequired validates the Bearer JWT and injects user claims into the context.
// Responds with 401 if the token is missing or invalid.
func AuthRequired(jwtSecret func() string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := auth.Verify(tokenStr, jwtSecret())
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(contextKeyUserID, claims.UserID)
		c.Set(contextKeyUsername, claims.Username)
		c.Set(contextKeyFullName, claims.FullName)
		c.Set(contextKeyRole, claims.Role)
		c.Next()
	}
}

// AdminRequired validates that the authenticated user has an admin role.
// Responds with 403 Forbidden if the user is not an admin.
// Must be used after AuthRequired middleware.
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString(contextKeyRole)
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		c.Next()
	}
}
