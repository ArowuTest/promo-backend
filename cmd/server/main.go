// cmd/server/main.go

package main

import (
	"log"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/ArowuTest/promo-backend/internal/auth"
	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/handlers"
	"github.com/ArowuTest/promo-backend/internal/models"
)

func main() {
	// ─── 1) Load config & initialize DB + migrations ──────────────────────────
	appCfg := config.Load()
	db := config.InitDB(appCfg)
	models.Migrate(db)

	// ─── 2) Initialize authentication (e.g. set up JWT) ─────────────────────
	auth.Init(appCfg.JWTSecret)

	// ─── 3) Create Gin router & register CORS middleware ────────────────────
	r := gin.Default()

	// This must appear before you register any /api/v1 routes.
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"https://promo-admin-portal.vercel.app", // your Vercel front-end
			"http://localhost:3000",                 // for local dev/testing
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		// How long to cache preflight response in the client:
		MaxAge: 12 * time.Hour,
	}))

	// ─── 4) Register your API routes ─────────────────────────────────────────
	api := r.Group("/api/v1")
	{
		// Auth
		api.POST("/admin/login", handlers.Login)

		// Admin users (CRUD)
		users := api.Group("/admin/users")
		{
			users.POST("", handlers.CreateUser)
			users.GET("", handlers.ListUsers)
			users.GET("/:id", handlers.GetUser)
			users.PUT("/:id", handlers.UpdateUser)
			users.DELETE("/:id", handlers.DeleteUser)
		}

		// PrizeStructure CRUD
		ps := api.Group("/prize-structures")
		{
			ps.GET("", handlers.ListPrizeStructures)
			ps.GET("/:id", handlers.GetPrizeStructure)
			ps.POST("", handlers.CreatePrizeStructure)
			ps.PUT("/:id", handlers.UpdatePrizeStructure)
			ps.DELETE("/:id", handlers.DeletePrizeStructure)
		}

		// Draw endpoints
		draws := api.Group("/draws")
		{
			draws.GET("", handlers.ListDraws)
			draws.POST("/execute", handlers.ExecuteDraw)
			draws.POST("/rerun/:id", handlers.RerunDraw)
		}
	}

	// ─── 5) Start HTTP server on configured Port ───────────────────────────────
	if err := r.Run(":" + appCfg.Port); err != nil {
		log.Fatalf("🚨 server failed to start: %v", err)
	}
}
