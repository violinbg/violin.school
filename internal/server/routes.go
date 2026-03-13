package server

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/violinbg/violin.school/internal/captcha"
)

func registerRoutes(r *gin.Engine, db *sql.DB) {
	// NOTE: This captcha store is process-local memory. In multi-instance deployments,
	// captcha generation and registration verification should use shared storage.
	captchaStore := captcha.NewStore()

	api := r.Group("/api/v1")
	protected := api.Group("/")
	protected.Use(AuthRequired(func() string { return jwtSecret(db) }))
	admin := protected.Group("/")
	admin.Use(AdminRequired())

	registerHealthRoutes(api, db)
	registerSetupRoutes(api, db)
	registerAuthRoutes(api, protected, db, captchaStore)
	registerCoursesRoutes(protected, db)
	registerUserRoutes(admin, db)
	registerAdminRoutes(admin, db)
	registerAIAdminRoutes(admin, db)
}
