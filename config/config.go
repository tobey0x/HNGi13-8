package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                  string
	DatabaseURL           string
	JWTSecret             string
	GoogleClientID        string
	GoogleClientSecret    string
	GoogleCallbackURL     string
	PaystackSecretKey     string
	PaystackPublicKey     string
	FrontendURL           string
}

var AppConfig *Config

func LoadConfig() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	AppConfig = &Config{
		Port:                  getEnv("PORT", "8080"),
		DatabaseURL:           getEnv("DATABASE_URL", ""),
		JWTSecret:             getEnv("JWT_SECRET", ""),
		GoogleClientID:        getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleCallbackURL:     getEnv("GOOGLE_CALLBACK_URL", ""),
		PaystackSecretKey:     getEnv("PAYSTACK_SECRET_KEY", ""),
		PaystackPublicKey:     getEnv("PAYSTACK_PUBLIC_KEY", ""),
		FrontendURL:           getEnv("FRONTEND_URL", "http://localhost:3000"),
	}

	validateConfig()
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func validateConfig() {
	if AppConfig.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if AppConfig.JWTSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	if AppConfig.GoogleClientID == "" {
		log.Fatal("GOOGLE_CLIENT_ID is required")
	}
	if AppConfig.GoogleClientSecret == "" {
		log.Fatal("GOOGLE_CLIENT_SECRET is required")
	}
	if AppConfig.PaystackSecretKey == "" {
		log.Fatal("PAYSTACK_SECRET_KEY is required")
	}
}
