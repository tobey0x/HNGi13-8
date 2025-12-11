package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"wallet-service/database"
	"wallet-service/models"
	"wallet-service/utils"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware handles both JWT and API key authentication
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			// Handle both "Bearer token" and just "token" formats
			token := strings.TrimPrefix(authHeader, "Bearer ")
			token = strings.TrimSpace(token)
			claims, err := utils.ValidateJWT(token)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "Invalid or expired token",
					"details": err.Error(),
				})
				c.Abort()
				return
			}

			c.Set("user_id", claims.UserID)
			c.Set("email", claims.Email)
			c.Set("auth_type", "jwt")
			c.Next()
			return
		}

		apiKey := c.GetHeader("x-api-key")
		if apiKey != "" {
			// Hash the incoming key to compare with stored hash
			hashedKey := utils.HashAPIKey(apiKey)
			
			var key models.APIKey
			if err := database.DB.Where("key = ? AND is_active = ?", hashedKey, true).First(&key).Error; err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
				c.Abort()
				return
			}

			if key.IsExpired() {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "API key has expired"})
				c.Abort()
				return
			}

			var permissions []string
			if err := json.Unmarshal([]byte(key.Permissions), &permissions); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse API key permissions"})
				c.Abort()
				return
			}

			c.Set("user_id", key.UserID)
			c.Set("auth_type", "api_key")
			c.Set("api_key_id", key.ID)
			c.Set("permissions", permissions)
			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		c.Abort()
	}
}

// RequirePermission checks if the request has the required permission
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authType, exists := c.Get("auth_type")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		if authType == "jwt" {
			c.Next()
			return
		}

		if authType == "api_key" {
			permissions, exists := c.Get("permissions")
			if !exists {
				c.JSON(http.StatusForbidden, gin.H{"error": "No permissions found"})
				c.Abort()
				return
			}

			permList, ok := permissions.([]string)
			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid permissions format"})
				c.Abort()
				return
			}

			hasPermission := false
			for _, p := range permList {
				if p == permission {
					hasPermission = true
					break
				}
			}

			if !hasPermission {
				c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
				c.Abort()
				return
			}

			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication type"})
		c.Abort()
	}
}

// RateLimitByAPIKey implements simple rate limiting for API keys
func RateLimitByAPIKey() gin.HandlerFunc {
	type rateLimitData struct {
		count     int
		resetTime time.Time
	}
	
	cache := make(map[string]*rateLimitData)
	
	return func(c *gin.Context) {
		apiKeyID, exists := c.Get("api_key_id")
		if !exists {
			c.Next()
			return
		}

		keyID := apiKeyID.(string)
		now := time.Now()

		if data, exists := cache[keyID]; exists {
			if now.After(data.resetTime) {
				cache[keyID] = &rateLimitData{count: 1, resetTime: now.Add(time.Minute)}
			} else {
				if data.count >= 100 {
					c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
					c.Abort()
					return
				}
				data.count++
			}
		} else {
			cache[keyID] = &rateLimitData{count: 1, resetTime: now.Add(time.Minute)}
		}

		c.Next()
	}
}
