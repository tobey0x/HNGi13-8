package handlers

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"wallet-service/config"
	"wallet-service/database"
	"wallet-service/models"
	"wallet-service/services"
	"wallet-service/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var paystackService = services.NewPaystackService()

type DepositRequest struct {
	Amount int64 `json:"amount" binding:"required,gt=0"`
}

func InitiateDeposit(c *gin.Context) {
	userID, _ := c.Get("user_id")
	email, _ := c.Get("email")

	var req DepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount is required and must be greater than 0"})
		return
	}

	// Get user's wallet
	var wallet models.Wallet
	if err := database.DB.Where("user_id = ?", userID).First(&wallet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}

	// Generate unique reference
	reference := utils.GenerateReference()

	// Create pending transaction
	transaction := models.Transaction{
		UserID:    userID.(string),
		Type:      models.TransactionTypeDeposit,
		Amount:    req.Amount,
		Status:    models.TransactionStatusPending,
		Reference: reference,
	}

	if err := database.DB.Create(&transaction).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create transaction"})
		return
	}

	// Initialize Paystack transaction
	emailStr := email.(string)
	result, err := paystackService.InitializeTransaction(emailStr, req.Amount, reference)
	if err != nil {
		log.Println("Paystack initialization error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reference":         reference,
		"authorization_url": result.Data.AuthorizationURL,
	})
}

type PaystackWebhookEvent struct {
	Event string `json:"event"`
	Data  struct {
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Status    string `json:"status"`
	} `json:"data"`
}

func PaystackWebhook(c *gin.Context) {
	// Verify Paystack signature
	signature := c.GetHeader("x-paystack-signature")
	if signature == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing signature"})
		return
	}

	// Read body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Verify signature
	if !verifyPaystackSignature(body, signature) {
		log.Println("Invalid Paystack signature")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
		return
	}

	// Parse webhook event
	var event PaystackWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Only process charge.success events
	if event.Event != "charge.success" {
		c.JSON(http.StatusOK, gin.H{"status": true})
		return
	}

	// Process the transaction
	if err := processSuccessfulDeposit(event.Data.Reference, event.Data.Amount); err != nil {
		log.Println("Failed to process deposit:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process deposit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": true})
}

func verifyPaystackSignature(body []byte, signature string) bool {
	mac := hmac.New(sha512.New, []byte(config.AppConfig.PaystackSecretKey))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func processSuccessfulDeposit(reference string, amount int64) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// Find transaction
		var transaction models.Transaction
		if err := tx.Where("reference = ?", reference).First(&transaction).Error; err != nil {
			return err
		}

		// Check if already processed (idempotency)
		if transaction.Status == models.TransactionStatusSuccess {
			log.Println("Transaction already processed:", reference)
			return nil
		}

		// Update transaction status
		transaction.Status = models.TransactionStatusSuccess
		if err := tx.Save(&transaction).Error; err != nil {
			return err
		}

		// Update wallet balance
		var wallet models.Wallet
		if err := tx.Where("user_id = ?", transaction.UserID).First(&wallet).Error; err != nil {
			return err
		}

		wallet.Balance += amount
		if err := tx.Save(&wallet).Error; err != nil {
			return err
		}

		log.Printf("Deposit processed: %s, Amount: %d, New Balance: %d", reference, amount, wallet.Balance)
		return nil
	})
}

func GetDepositStatus(c *gin.Context) {
	reference := c.Param("reference")

	var transaction models.Transaction
	if err := database.DB.Where("reference = ?", reference).First(&transaction).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reference": transaction.Reference,
		"status":    transaction.Status,
		"amount":    transaction.Amount,
	})
}

func GetWalletBalance(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var wallet models.Wallet
	if err := database.DB.Where("user_id = ?", userID).First(&wallet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"balance": wallet.Balance,
	})
}

func GetTransactionHistory(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var transactions []models.Transaction
	if err := database.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&transactions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch transactions"})
		return
	}

	type TransactionResponse struct {
		Type   string `json:"type"`
		Amount int64  `json:"amount"`
		Status string `json:"status"`
	}

	var response []TransactionResponse
	for _, tx := range transactions {
		response = append(response, TransactionResponse{
			Type:   string(tx.Type),
			Amount: tx.Amount,
			Status: string(tx.Status),
		})
	}

	c.JSON(http.StatusOK, response)
}

type TransferRequest struct {
	WalletNumber string `json:"wallet_number" binding:"required"`
	Amount       int64  `json:"amount" binding:"required,gt=0"`
}

func TransferFunds(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Execute transfer in transaction
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// Get sender's wallet
		var senderWallet models.Wallet
		if err := tx.Where("user_id = ?", userID).First(&senderWallet).Error; err != nil {
			return err
		}

		// Check sufficient balance
		if senderWallet.Balance < req.Amount {
			return fmt.Errorf("insufficient balance")
		}

		// Get recipient's wallet
		var recipientWallet models.Wallet
		if err := tx.Where("wallet_number = ?", req.WalletNumber).First(&recipientWallet).Error; err != nil {
			return fmt.Errorf("recipient wallet not found")
		}

		// Check if trying to transfer to self
		if senderWallet.ID == recipientWallet.ID {
			return fmt.Errorf("cannot transfer to your own wallet")
		}

		// Deduct from sender
		senderWallet.Balance -= req.Amount
		if err := tx.Save(&senderWallet).Error; err != nil {
			return err
		}

		// Add to recipient
		recipientWallet.Balance += req.Amount
		if err := tx.Save(&recipientWallet).Error; err != nil {
			return err
		}

		// Record sender's transaction
		senderTx := models.Transaction{
			UserID:            userID.(string),
			Type:              models.TransactionTypeTransfer,
			Amount:            req.Amount,
			Status:            models.TransactionStatusSuccess,
			Reference:         utils.GenerateReference(),
			RecipientWalletID: &recipientWallet.ID,
		}
		if err := tx.Create(&senderTx).Error; err != nil {
			return err
		}

		// Record recipient's transaction
		recipientTx := models.Transaction{
			UserID:         recipientWallet.UserID,
			Type:           models.TransactionTypeCredit,
			Amount:         req.Amount,
			Status:         models.TransactionStatusSuccess,
			Reference:      utils.GenerateReference(),
			SenderWalletID: &senderWallet.ID,
		}
		if err := tx.Create(&recipientTx).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		if err.Error() == "insufficient balance" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient balance"})
			return
		}
		if err.Error() == "recipient wallet not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Recipient wallet not found"})
			return
		}
		if err.Error() == "cannot transfer to your own wallet" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot transfer to your own wallet"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transfer failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Transfer completed",
	})
}
