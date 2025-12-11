package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"wallet-service/database"
	"wallet-service/models"

	"github.com/gin-gonic/gin"
)

// responseWriter wraps gin.ResponseWriter to capture response
type responseWriter struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// IdempotencyMiddleware ensures requests with the same idempotency key are processed only once
func IdempotencyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		idempotencyKey := c.GetHeader("X-Idempotency-Key")
		
		// If no idempotency key provided, proceed without idempotency check
		if idempotencyKey == "" {
			c.Next()
			return
		}

		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		// Read request body for comparison
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Hash the idempotency key with user_id and path for uniqueness
		hasher := sha256.New()
		hasher.Write([]byte(userID.(string)))
		hasher.Write([]byte(c.Request.URL.Path))
		hasher.Write([]byte(idempotencyKey))
		keyHash := hex.EncodeToString(hasher.Sum(nil))

		// Check if this idempotency key exists and hasn't expired
		var existingKey models.IdempotencyKey
		err := database.DB.Where("key = ? AND user_id = ?", keyHash, userID).First(&existingKey).Error
		
		if err == nil {
			// Key exists - check if expired
			if existingKey.IsExpired() {
				// Expired, delete it and allow new request
				database.DB.Delete(&existingKey)
			} else {
				// Not expired - return cached response
				c.Data(existingKey.ResponseCode, "application/json", []byte(existingKey.ResponseBody))
				c.Abort()
				return
			}
		}

		// Wrap response writer to capture response
		responseBodyWriter := &responseWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
			statusCode:     http.StatusOK,
		}
		c.Writer = responseBodyWriter

		// Process the request
		c.Next()

		// Store the idempotency key with response (only for successful requests)
		if responseBodyWriter.statusCode >= 200 && responseBodyWriter.statusCode < 300 {
			newKey := models.IdempotencyKey{
				Key:          keyHash,
				UserID:       userID.(string),
				RequestPath:  c.Request.URL.Path,
				RequestBody:  string(requestBody),
				ResponseCode: responseBodyWriter.statusCode,
				ResponseBody: responseBodyWriter.body.String(),
				ExpiresAt:    time.Now().Add(24 * time.Hour), // Keys expire after 24 hours
			}

			database.DB.Create(&newKey)

			// Clean up expired keys periodically (simple approach)
			go func() {
				database.DB.Where("expires_at < ?", time.Now()).Delete(&models.IdempotencyKey{})
			}()
		}
	}
}

// GenerateIdempotencyKey generates a unique idempotency key based on request data
func GenerateIdempotencyKey(data interface{}) string {
	jsonData, _ := json.Marshal(data)
	hasher := sha256.New()
	hasher.Write(jsonData)
	hasher.Write([]byte(time.Now().Format("2006-01-02-15"))) // Changes hourly
	return hex.EncodeToString(hasher.Sum(nil))
}
