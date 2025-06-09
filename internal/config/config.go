package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var Cfg *AppConfig

// AppConfig holds all environment variables.
type AppConfig struct {
	Port            string
	DBHost          string
	DBPort          string
	DBUser          string
	DBName          string
	DBPassword      string
	DBSSLMode       string
	JWTSecret       string
	FrontendURL     string
	PosthogAPIKey   string // This field is restored
	PosthogEndpoint string // This field is restored
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
		DBSSLMode:       os.Getenv("DB_SSLMODE"),
		JWTSecret:       os.Getenv("JWT_SECRET_KEY"),
		FrontendURL:     os.Getenv("FRONTEND_URL"),
		PosthogAPIKey:   os.Getenv("POSTHOG_API_KEY"),           // This line is restored
		PosthogEndpoint: os.Getenv("POSTHOG_INSTANCE_ADDRESS"), // This line is restored
	}
	if Cfg.Port == "" {
		Cfg.Port = "8080"
	}
	if Cfg.DBSSLMode == "" {
		Cfg.DBSSLMode = "disable"
	}
	return Cfg
}

var DB *gorm.DB

// InitDB has been updated to include a detailed logger.
func InitDB(c *AppConfig) *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		c.DBHost, c.DBUser, c.DBPassword, c.DBName, c.DBPort, c.DBSSLMode,
	)

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Info,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		panic("failed to connect database: " + err.Error())
	}
	DB = db
	return db
}