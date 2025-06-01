package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var Cfg *AppConfig

// AppConfig holds all environment variables.
type AppConfig struct {
	Port              string
	DBHost            string
	DBPort            string
	DBUser            string
	DBName            string
	DBPassword        string
	JWTSecret         string
	FrontendURL       string
	PosthogAPIKey     string
	PosthogEndpoint   string
}

// CORSMiddleware allows cross‐origin requests from your frontend.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", Cfg.FrontendURL)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// Load reads environment variables (and .env if present)
func Load() *AppConfig {
	_ = godotenv.Load()

	Cfg = &AppConfig{
		Port:            os.Getenv("PORT"),
		DBHost:          os.Getenv("DB_HOST"),
		DBPort:          os.Getenv("DB_PORT"),
		DBUser:          os.Getenv("DB_USER"),
		DBName:          os.Getenv("DB_NAME"),
		DBPassword:      os.Getenv("DB_PASSWORD"),
		JWTSecret:       os.Getenv("JWT_SECRET_KEY"),
		FrontendURL:     os.Getenv("FRONTEND_URL"),
		PosthogAPIKey:   os.Getenv("POSTHOG_API_KEY"),
		PosthogEndpoint: os.Getenv("POSTHOG_INSTANCE_ADDRESS"),
	}
	if Cfg.Port == "" {
		Cfg.Port = "8080"
	}
	return Cfg
}

var DB *gorm.DB

// InitDB opens the PostgreSQL connection via GORM.
func InitDB(c *AppConfig) *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		c.DBHost, c.DBUser, c.DBPassword, c.DBName, c.DBPort,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database: " + err.Error())
	}
	DB = db
	return db
}

// CloseDB closes the underlying SQL connection.
func CloseDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
