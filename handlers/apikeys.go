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
	Name        string   `json:"name" binding:"required" example:"production-api"`
	Permissions []string `json:"permissions" binding:"required" example:"deposit,transfer,read"`
	Expiry      string   `json:"expiry" binding:"required" example:"1D"`
}

type CreateAPIKeyResponse struct {
	APIKey    string `json:"api_key" example:"sk_live_xxxxx"`
	ExpiresAt string `json:"expires_at" example:"2025-12-11T12:00:00Z"`
}

type RolloverAPIKeyRequest struct {
	ExpiredKeyID string `json:"expired_key_id" binding:"required" example:"uuid-here"`
	Expiry       string `json:"expiry" binding:"required" example:"1M"`
}

// CreateAPIKey godoc
// @Summary Create a new API key
// @Description Create a new API key with specific permissions and expiry (max 5 active keys per user)
// @Tags API Keys
// @Accept json
// @Produce json
// @Param request body CreateAPIKeyRequest true "API key details. Expiry: 1H, 1D, 1M, 1Y"
// @Success 201 {object} CreateAPIKeyResponse
// @Failure 400 {object} map[string]interface{} "Bad request or max keys reached"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Router /keys/create [post]
func CreateAPIKey(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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

	var activeCount int64
	database.DB.Model(&models.APIKey{}).
		Where("user_id = ? AND is_active = ? AND expires_at > NOW()", userID, true).
		Count(&activeCount)
	
	if activeCount >= 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum of 5 active API keys allowed"})
		return
	}

	expiresAt, err := utils.ParseExpiry(req.Expiry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry format. Use 1H, 1D, 1M, or 1Y"})
		return
	}

	keyValue, err := utils.GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate API key"})
		return
	}

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

// RolloverAPIKey godoc
// @Summary Rollover an expired API key
// @Description Create a new API key with the same permissions as an expired key
// @Tags API Keys
// @Accept json
// @Produce json
// @Param request body RolloverAPIKeyRequest true "Expired key ID and new expiry"
// @Success 201 {object} CreateAPIKeyResponse
// @Failure 400 {object} map[string]interface{} "Bad request or key not expired"
// @Failure 404 {object} map[string]interface{} "API key not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Router /keys/rollover [post]
func RolloverAPIKey(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req RolloverAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var expiredKey models.APIKey
	if err := database.DB.Where("id = ? AND user_id = ?", req.ExpiredKeyID, userID).First(&expiredKey).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		return
	}

	if !expiredKey.IsExpired() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key is not expired yet"})
		return
	}

	var activeCount int64
	database.DB.Model(&models.APIKey{}).
		Where("user_id = ? AND is_active = ? AND expires_at > NOW()", userID, true).
		Count(&activeCount)
	
	if activeCount >= 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum of 5 active API keys allowed"})
		return
	}

	expiresAt, err := utils.ParseExpiry(req.Expiry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry format. Use 1H, 1D, 1M, or 1Y"})
		return
	}

	keyValue, err := utils.GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate API key"})
		return
	}

	newKey := models.APIKey{
		UserID:      userID.(string),
		Name:        expiredKey.Name,
		Key:         keyValue,
		Permissions: expiredKey.Permissions,
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

type APIKeyResponse struct {
	ID          string `json:"id" example:"uuid-here"`
	Name        string `json:"name" example:"production-api"`
	Permissions string `json:"permissions" example:"[\"deposit\",\"transfer\",\"read\"]"`
	ExpiresAt   string `json:"expires_at" example:"2025-12-11T12:00:00Z"`
	IsActive    bool   `json:"is_active" example:"true"`
	IsExpired   bool   `json:"is_expired" example:"false"`
}

// ListAPIKeys godoc
// @Summary List all API keys
// @Description Get all API keys for the authenticated user (actual key values are not exposed)
// @Tags API Keys
// @Produce json
// @Success 200 {array} APIKeyResponse
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Router /keys/list [get]
func ListAPIKeys(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var apiKeys []models.APIKey
	if err := database.DB.Where("user_id = ?", userID).Find(&apiKeys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch API keys"})
		return
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

// RevokeAPIKey godoc
// @Summary Revoke an API key
// @Description Deactivate an API key by its ID
// @Tags API Keys
// @Produce json
// @Param id path string true "API Key ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{} "API key not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Router /keys/{id} [delete]
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
