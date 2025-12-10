package main

import (
	"log"
	"time"
	"wallet-service/config"
	"wallet-service/database"
	"wallet-service/handlers"
	"wallet-service/middleware"

	_ "wallet-service/docs"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title Wallet Service API
// @version 1.0
// @description Backend wallet service with Paystack integration, Google OAuth, and API key management
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@walletservice.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name x-api-key
// @description API Key for service-to-service authentication

func main() {
	config.LoadConfig()
	database.Connect()
	database.Migrate()
	handlers.InitGoogleOAuth()

	router := gin.Default()

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{config.AppConfig.FrontendURL, "http://localhost:3000", "http://localhost:3001"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key", "x-paystack-signature"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	auth := router.Group("/auth")
	{
		auth.GET("/google", handlers.GoogleLogin)
		auth.GET("/google/callback", handlers.GoogleCallback)
	}

	keys := router.Group("/keys")
	keys.Use(middleware.AuthMiddleware())
	{
		keys.POST("/create", handlers.CreateAPIKey)
		keys.POST("/rollover", handlers.RolloverAPIKey)
		keys.GET("/list", handlers.ListAPIKeys)
		keys.DELETE("/:id", handlers.RevokeAPIKey)
	}

	wallet := router.Group("/wallet")
	{
		wallet.POST("/deposit",
			middleware.AuthMiddleware(),
			middleware.RequirePermission("deposit"),
			handlers.InitiateDeposit,
		)

		wallet.POST("/paystack/webhook", handlers.PaystackWebhook)

		wallet.GET("/deposit/:reference/status",
			middleware.AuthMiddleware(),
			handlers.GetDepositStatus,
		)

		wallet.GET("/balance",
			middleware.AuthMiddleware(),
			middleware.RequirePermission("read"),
			handlers.GetWalletBalance,
		)

		wallet.GET("/transactions",
			middleware.AuthMiddleware(),
			middleware.RequirePermission("read"),
			handlers.GetTransactionHistory,
		)

		wallet.POST("/transfer",
			middleware.AuthMiddleware(),
			middleware.RequirePermission("transfer"),
			handlers.TransferFunds,
		)
	}

	port := config.AppConfig.Port
	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
