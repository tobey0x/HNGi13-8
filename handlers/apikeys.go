package handlers

import (
	"encoding/json"
	"net/http"
	"wallet-service/database"
	"wallet-service/models"
	"wallet-service/utils"

	"github.com/gin-gonic/gin"
)

type CreateAPIKeyRequest struct {
	Name        string   `json:"name" binding:"required"`
	Permissions []string `json:"permissions" binding:"required"`
	Expiry      string   `json:"expiry" binding:"required"`
}

type RolloverAPIKeyRequest struct {
	ExpiredKeyID string `json:"expired_key_id" binding:"required"`
	Expiry       string `json:"expiry" binding:"required"`
}

func CreateAPIKey(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate permissions
	validPermissions := map[string]bool{
		"deposit":  true,
		"transfer": true,
		"read":     true,
	}
	
	for _, perm := range req.Permissions {
		if !validPermissions[perm] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid permission: " + perm})
			return
		}
	}

	// Check active API keys limit (max 5)
	var activeCount int64
	database.DB.Model(&models.APIKey{}).
		Where("user_id = ? AND is_active = ? AND expires_at > NOW()", userID, true).
		Count(&activeCount)
	
	if activeCount >= 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum of 5 active API keys allowed"})
		return
	}

	// Parse expiry
	expiresAt, err := utils.ParseExpiry(req.Expiry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry format. Use 1H, 1D, 1M, or 1Y"})
		return
	}

	// Generate API key
	keyValue, err := utils.GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate API key"})
		return
	}

	// Store permissions as JSON
	permissionsJSON, _ := json.Marshal(req.Permissions)

	apiKey := models.APIKey{
		UserID:      userID.(string),
		Name:        req.Name,
		Key:         keyValue,
		Permissions: string(permissionsJSON),
		ExpiresAt:   expiresAt,
		IsActive:    true,
	}

	if err := database.DB.Create(&apiKey).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create API key"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"api_key":    keyValue,
		"expires_at": expiresAt,
	})
}

func RolloverAPIKey(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req RolloverAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find the expired key
	var expiredKey models.APIKey
	if err := database.DB.Where("id = ? AND user_id = ?", req.ExpiredKeyID, userID).First(&expiredKey).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		return
	}

	// Check if key is actually expired
	if !expiredKey.IsExpired() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key is not expired yet"})
		return
	}

	// Check active API keys limit (max 5)
	var activeCount int64
	database.DB.Model(&models.APIKey{}).
		Where("user_id = ? AND is_active = ? AND expires_at > NOW()", userID, true).
		Count(&activeCount)
	
	if activeCount >= 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum of 5 active API keys allowed"})
		return
	}

	// Parse new expiry
	expiresAt, err := utils.ParseExpiry(req.Expiry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry format. Use 1H, 1D, 1M, or 1Y"})
		return
	}

	// Generate new API key
	keyValue, err := utils.GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate API key"})
		return
	}

	// Create new key with same permissions
	newKey := models.APIKey{
		UserID:      userID.(string),
		Name:        expiredKey.Name,
		Key:         keyValue,
		Permissions: expiredKey.Permissions, // Reuse same permissions
		ExpiresAt:   expiresAt,
		IsActive:    true,
	}

	if err := database.DB.Create(&newKey).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new API key"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"api_key":    keyValue,
		"expires_at": expiresAt,
	})
}

func ListAPIKeys(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var apiKeys []models.APIKey
	if err := database.DB.Where("user_id = ?", userID).Find(&apiKeys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch API keys"})
		return
	}

	// Don't expose the actual key value
	type APIKeyResponse struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Permissions string `json:"permissions"`
		ExpiresAt   string `json:"expires_at"`
		IsActive    bool   `json:"is_active"`
		IsExpired   bool   `json:"is_expired"`
	}

	var response []APIKeyResponse
	for _, key := range apiKeys {
		response = append(response, APIKeyResponse{
			ID:          key.ID,
			Name:        key.Name,
			Permissions: key.Permissions,
			ExpiresAt:   key.ExpiresAt.Format("2006-01-02T15:04:05Z"),
			IsActive:    key.IsActive,
			IsExpired:   key.IsExpired(),
		})
	}

	c.JSON(http.StatusOK, response)
}

func RevokeAPIKey(c *gin.Context) {
	userID, _ := c.Get("user_id")
	keyID := c.Param("id")

	result := database.DB.Model(&models.APIKey{}).
		Where("id = ? AND user_id = ?", keyID, userID).
		Update("is_active", false)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke API key"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API key revoked successfully"})
}
