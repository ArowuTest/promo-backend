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
	// ─── 1) Load config & initialize DB and migrations ────────────────────────────
	appCfg := config.Load()
	db := config.InitDB(appCfg)
	models.Migrate(db)

	// ─── 2) Initialize authentication (e.g. JWT secret setup) ───────────────────
	auth.Init(appCfg.JWTSecret)

	// ─── 3) Create Gin router and register CORS middleware ───────────────────────
	r := gin.Default()
	
	// AllowOrigins should include your Vercel front-end domain and localhost for local dev.
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"https://promo-admin-portal.vercel.app", // production front-end
			"http://localhost:3000",                 // local dev front-end
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// ─── 4) Register API routes ─────────────────────────────────────────────────
	api := r.Group("/api/v1")
	{
		// Auth
		api.POST("/admin/login", handlers.Login)

		// Admin users (CRUD)
		users := api.Group("/admin/users")
		{
			users.POST("", handlers.CreateUser)      // create
			users.GET("", handlers.ListUsers)        // list
			users.GET("/:id", handlers.GetUser)      // get by ID
			users.PUT("/:id", handlers.UpdateUser)   // update
			users.DELETE("/:id", handlers.DeleteUser) // delete
		}

		// PrizeStructure CRUD
		ps := api.Group("/prize-structures")
		{
			ps.GET("", handlers.ListPrizeStructures)         // list all
			ps.GET("/:id", handlers.GetPrizeStructure)       // get one
			ps.POST("", handlers.CreatePrizeStructure)       // create
			ps.PUT("/:id", handlers.UpdatePrizeStructure)    // update
			ps.DELETE("/:id", handlers.DeletePrizeStructure) // delete
		}

		// Draw endpoints
		draws := api.Group("/draws")
		{
			draws.GET("", handlers.ListDraws)            // list past draws
			draws.POST("/execute", handlers.ExecuteDraw) // execute a brand‐new draw
			draws.POST("/rerun/:id", handlers.RerunDraw) // rerun an existing draw
		}
	}

	// ─── 5) Start the HTTP server on the configured port ─────────────────────────
	if err := r.Run(":" + appCfg.Port); err != nil {
		log.Fatalf("🚨 server failed to start: %v", err)
	}
}
