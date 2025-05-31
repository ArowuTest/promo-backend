// cmd/server/main.go

package main

import (
	"github.com/gin-gonic/gin"

	"github.com/ArowuTest/promo-backend/internal/auth"
	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/handlers"
	"github.com/ArowuTest/promo-backend/internal/models"
)

func main() {
	// 1) Load config & initialize DB
	appCfg := config.Load()
	db := config.InitDB(appCfg)
	models.Migrate(db)
	auth.Init(appCfg.JWTSecret)

	// 2) Setup Gin router
	r := gin.Default()
	r.Use(config.CORSMiddleware())

	api := r.Group("/api/v1")
	{
		// Public login endpoint
		api.POST("/admin/login", handlers.Login)

		// Admin‐user CRUD (protected: only SUPERADMIN can create/update/delete, 
		// but LIST and GET might be allowed for Admin role).
		users := api.Group("/admin/users")
		users.Use(handlers.RequireAuth(models.RoleSuperAdmin))
		{
			users.POST("", handlers.CreateUser)
			users.GET("", handlers.ListUsers)
			users.GET("/:id", handlers.GetUser)
			users.PUT("/:id", handlers.UpdateUser)
			users.DELETE("/:id", handlers.DeleteUser)
		}

		// Draw endpoints:
		draws := api.Group("/draws")
		// Only SUPERADMIN may execute or rerun a draw
		draws.Use(handlers.RequireAuth(models.RoleSuperAdmin))
		{
			draws.GET("", handlers.ListDraws)            // can be viewed by anyone with a token (Admin+)
			draws.POST("/execute", handlers.ExecuteDraw) // superadmin only
			draws.POST("/rerun/:id", handlers.RerunDraw) // superadmin only
		}
	}

	// 3) Run server
	r.Run(":" + appCfg.Port)
}
