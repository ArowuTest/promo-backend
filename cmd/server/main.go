package main

import (
	"github.com/gin-gonic/gin"

	"github.com/ArowuTest/promo-backend/internal/auth"
	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/handlers"
	"github.com/ArowuTest/promo-backend/internal/models"
)

func main() {
	// Load config & init
	appCfg := config.Load()
	db := config.InitDB(appCfg)
	models.Migrate(db)
	auth.Init(appCfg.JWTSecret) // pass in JWT secret

	// Setup router
	r := gin.Default()
	r.Use(config.CORSMiddleware())

	api := r.Group("/api/v1")
	{
		// Auth
		api.POST("/admin/login", handlers.Login)

		// Admin users (CRUD)
		users := api.Group("/admin/users")
		{
			users.POST("", handlers.CreateUser)                             // create
			users.GET("", handlers.ListUsers)                                // list
			users.GET("/:id", handlers.GetUser)                              // get by ID
			users.PUT("/:id", handlers.UpdateUser)                           // update
			users.DELETE("/:id", handlers.DeleteUser)                        // delete
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
			draws.GET("", handlers.ListDraws)              // list past draws
			draws.POST("/execute", handlers.ExecuteDraw)   // execute a brand‐new draw
			draws.POST("/rerun/:id", handlers.RerunDraw)   // rerun an existing draw
		}
	}

	// Start the HTTP server (port from env or default)
	r.Run(":" + appCfg.Port)
}
