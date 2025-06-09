package main

import (
	"log"
	"time"

	"github.com/ArowuTest/promo-backend/internal/auth"
	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/handlers"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	appCfg := config.Load()
	db := config.InitDB(appCfg)
	models.Migrate(db)
	auth.Init(appCfg.JWTSecret)

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{appCfg.FrontendURL},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	apiV1 := r.Group("/api/v1")
	{
		apiV1.POST("/admin/login", handlers.Login)

		authGroup := apiV1.Group("/")
		authGroup.Use(handlers.RequireAuth())

		userRoutes := authGroup.Group("/admin/users")
		userRoutes.Use(handlers.RequireAuth(models.RoleSuperAdmin))
		{
			userRoutes.POST("", handlers.CreateUser)
			userRoutes.GET("", handlers.ListUsers)
			userRoutes.GET("/:id", handlers.GetUser)
			userRoutes.PUT("/:id", handlers.UpdateUser)
			userRoutes.DELETE("/:id", handlers.DeleteUser)
		}

		prizeRoutes := authGroup.Group("/prize-structures")
		prizeRoutes.Use(handlers.RequireAuth(models.RoleSuperAdmin, models.RoleAdmin))
		{
			prizeRoutes.POST("", handlers.CreatePrizeStructure)
			prizeRoutes.GET("", handlers.ListPrizeStructures)
			prizeRoutes.GET("/:id", handlers.GetPrizeStructure)
			prizeRoutes.PUT("/:id", handlers.UpdatePrizeStructure)
			prizeRoutes.DELETE("/:id", handlers.DeletePrizeStructure)
		}

		drawRoutes := authGroup.Group("/draws")
		{
			drawRoutes.GET("", handlers.RequireAuth(models.RoleSuperAdmin, models.RoleAdmin, models.RoleSeniorUser), handlers.ListDraws)
			drawRoutes.GET("/:id/winners", handlers.RequireAuth(models.RoleSuperAdmin, models.RoleAdmin, models.RoleSeniorUser), handlers.ListWinners)
			drawRoutes.POST("/execute", handlers.RequireAuth(models.RoleSuperAdmin), handlers.ExecuteDraw)
			drawRoutes.POST("/rerun/:id", handlers.RequireAuth(models.RoleSuperAdmin), handlers.RerunDraw)
		}
	}

	log.Printf("Starting server on port %s", appCfg.Port)
	if err := r.Run(":" + appCfg.Port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}