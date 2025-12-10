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
	Amount int64 `json:"amount" binding:"required,gt=0" example:"5000"`
}

type DepositResponse struct {
	Reference        string `json:"reference" example:"TXN_1234567890"`
	AuthorizationURL string `json:"authorization_url" example:"https://checkout.paystack.com/..."`
}

// InitiateDeposit godoc
// @Summary Initiate wallet deposit
// @Description Initialize a Paystack transaction for depositing money into wallet
// @Tags Wallet
// @Accept json
// @Produce json
// @Param request body DepositRequest true "Deposit amount in kobo (100 kobo = ₦1)"
// @Success 200 {object} DepositResponse
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 404 {object} map[string]interface{} "Wallet not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Security ApiKeyAuth
// @Router /wallet/deposit [post]
func InitiateDeposit(c *gin.Context) {
	userID, _ := c.Get("user_id")
	email, _ := c.Get("email")

	var req DepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount is required and must be greater than 0"})
		return
	}

	var wallet models.Wallet
	if err := database.DB.Where("user_id = ?", userID).First(&wallet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}

	reference := utils.GenerateReference()

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

// PaystackWebhook godoc
// @Summary Paystack webhook handler
// @Description Receives and processes payment notifications from Paystack (signature verified)
// @Tags Wallet
// @Accept json
// @Produce json
// @Param x-paystack-signature header string true "Paystack signature"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 401 {object} map[string]interface{} "Invalid signature"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /wallet/paystack/webhook [post]
func PaystackWebhook(c *gin.Context) {
	signature := c.GetHeader("x-paystack-signature")
	if signature == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing signature"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	if !verifyPaystackSignature(body, signature) {
		log.Println("Invalid Paystack signature")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
		return
	}

	var event PaystackWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if event.Event != "charge.success" {
		c.JSON(http.StatusOK, gin.H{"status": true})
		return
	}

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
		var transaction models.Transaction
		if err := tx.Where("reference = ?", reference).First(&transaction).Error; err != nil {
			return err
		}

		if transaction.Status == models.TransactionStatusSuccess {
			log.Println("Transaction already processed:", reference)
			return nil
		}

		transaction.Status = models.TransactionStatusSuccess
		if err := tx.Save(&transaction).Error; err != nil {
			return err
		}

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

// GetDepositStatus godoc
// @Summary Get deposit transaction status
// @Description Manually check the status of a deposit transaction by reference
// @Tags Wallet
// @Produce json
// @Param reference path string true "Transaction reference"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{} "Transaction not found"
// @Security BearerAuth
// @Security ApiKeyAuth
// @Router /wallet/deposit/{reference}/status [get]
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

// GetWalletBalance godoc
// @Summary Get wallet balance
// @Description Retrieve the current balance of the authenticated user's wallet
// @Tags Wallet
// @Produce json
// @Success 200 {object} map[string]interface{} "Balance in kobo"
// @Failure 404 {object} map[string]interface{} "Wallet not found"
// @Security BearerAuth
// @Security ApiKeyAuth
// @Router /wallet/balance [get]
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

type TransactionResponse struct {
	Type   string `json:"type" example:"deposit"`
	Amount int64  `json:"amount" example:"5000"`
	Status string `json:"status" example:"success"`
}

// GetTransactionHistory godoc
// @Summary Get transaction history
// @Description Retrieve all transactions for the authenticated user
// @Tags Wallet
// @Produce json
// @Success 200 {array} TransactionResponse
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Security ApiKeyAuth
// @Router /wallet/transactions [get]
func GetTransactionHistory(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var transactions []models.Transaction
	if err := database.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&transactions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch transactions"})
		return
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
	WalletNumber string `json:"wallet_number" binding:"required" example:"1234567890123"`
	Amount       int64  `json:"amount" binding:"required,gt=0" example:"3000"`
}

// TransferFunds godoc
// @Summary Transfer funds to another wallet
// @Description Transfer money from authenticated user's wallet to another user's wallet
// @Tags Wallet
// @Accept json
// @Produce json
// @Param request body TransferRequest true "Transfer details"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{} "Bad request or insufficient balance"
// @Failure 404 {object} map[string]interface{} "Recipient wallet not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Security ApiKeyAuth
// @Router /wallet/transfer [post]
func TransferFunds(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var senderWallet models.Wallet
		if err := tx.Where("user_id = ?", userID).First(&senderWallet).Error; err != nil {
			return err
		}

		if senderWallet.Balance < req.Amount {
			return fmt.Errorf("insufficient balance")
		}

		var recipientWallet models.Wallet
		if err := tx.Where("wallet_number = ?", req.WalletNumber).First(&recipientWallet).Error; err != nil {
			return fmt.Errorf("recipient wallet not found")
		}

		if senderWallet.ID == recipientWallet.ID {
			return fmt.Errorf("cannot transfer to your own wallet")
		}

		senderWallet.Balance -= req.Amount
		if err := tx.Save(&senderWallet).Error; err != nil {
			return err
		}

		recipientWallet.Balance += req.Amount
		if err := tx.Save(&recipientWallet).Error; err != nil {
			return err
		}

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
