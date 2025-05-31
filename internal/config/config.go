// internal/config/config.go

package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var Cfg *AppConfig // ← global config pointer
var DB  *gorm.DB   // ← global GORM db handle

// AppConfig holds all env vars
type AppConfig struct {
	Port            string
	DBHost          string
	DBPort          string
	DBUser          string
	DBName          string
	DBPassword      string
	JWTSecret       string
	FrontendURL     string
	PosthogAPIKey   string
	PosthogEndpoint string
}

// CORSMiddleware sets CORS headers based on FRONTEND_URL
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", Cfg.FrontendURL)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// Load reads environment variables into AppConfig.
// It also sets the global `Cfg` pointer.
func Load() *AppConfig {
	_ = godotenv.Load() // ignore errors if no .env file
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

// InitDB opens a PostgreSQL connection (via GORM) based on AppConfig.
// It sets the package‐level variable `DB` and also returns it.
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

// CloseDB gracefully closes the underlying SQL connection.
func CloseDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
