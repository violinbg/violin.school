package server

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func registerCoursesRoutes(protected *gin.RouterGroup, db *sql.DB) {
	protected.GET("/courses", handleListCourses(db))
}

// courseResponse is the JSON representation sent to the client.
type courseResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Published   bool   `json:"published"`
	CreatedAt   string `json:"created_at"`
}

func handleListCourses(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(`
			SELECT id, title, description, published, created_at
			FROM courses WHERE published = 1 ORDER BY created_at DESC`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		courses := []courseResponse{}
		for rows.Next() {
			var r courseResponse
			if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.Published, &r.CreatedAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			courses = append(courses, r)
		}

		c.JSON(http.StatusOK, courses)
	}
}
