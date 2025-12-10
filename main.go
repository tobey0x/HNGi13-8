package main

import (
	"log"
	"wallet-service/config"
	"wallet-service/database"
	"wallet-service/handlers"
	"wallet-service/middleware"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	config.LoadConfig()

	// Connect to database
	database.Connect()
	database.Migrate()

	// Initialize Google OAuth
	handlers.InitGoogleOAuth()

	// Setup Gin router
	router := gin.Default()

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Authentication routes (no auth required)
	auth := router.Group("/auth")
	{
		auth.GET("/google", handlers.GoogleLogin)
		auth.GET("/google/callback", handlers.GoogleCallback)
	}

	// API Key management routes (requires JWT)
	keys := router.Group("/keys")
	keys.Use(middleware.AuthMiddleware())
	{
		keys.POST("/create", handlers.CreateAPIKey)
		keys.POST("/rollover", handlers.RolloverAPIKey)
		keys.GET("/list", handlers.ListAPIKeys)
		keys.DELETE("/:id", handlers.RevokeAPIKey)
	}

	// Wallet routes
	wallet := router.Group("/wallet")
	{
		// Deposit endpoints
		wallet.POST("/deposit",
			middleware.AuthMiddleware(),
			middleware.RequirePermission("deposit"),
			handlers.InitiateDeposit,
		)

		// Webhook endpoint (no auth - verified by signature)
		wallet.POST("/paystack/webhook", handlers.PaystackWebhook)

		// Deposit status (manual check)
		wallet.GET("/deposit/:reference/status",
			middleware.AuthMiddleware(),
			handlers.GetDepositStatus,
		)

		// Balance endpoint
		wallet.GET("/balance",
			middleware.AuthMiddleware(),
			middleware.RequirePermission("read"),
			handlers.GetWalletBalance,
		)

		// Transaction history
		wallet.GET("/transactions",
			middleware.AuthMiddleware(),
			middleware.RequirePermission("read"),
			handlers.GetTransactionHistory,
		)

		// Transfer endpoint
		wallet.POST("/transfer",
			middleware.AuthMiddleware(),
			middleware.RequirePermission("transfer"),
			handlers.TransferFunds,
		)
	}

	// Start server
	port := config.AppConfig.Port
	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
