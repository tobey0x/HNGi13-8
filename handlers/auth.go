package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"wallet-service/config"
	"wallet-service/database"
	"wallet-service/models"
	"wallet-service/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var googleOauthConfig *oauth2.Config

func InitGoogleOAuth() {
	googleOauthConfig = &oauth2.Config{
		ClientID:     config.AppConfig.GoogleClientID,
		ClientSecret: config.AppConfig.GoogleClientSecret,
		RedirectURL:  config.AppConfig.GoogleCallbackURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}

// GoogleLogin godoc
// @Summary Initiate Google OAuth login
// @Description Returns Google OAuth URL. For normal flow: open URL and sign in, you'll get token automatically. For testing in Swagger: add debug=true parameter to see the code first.
// @Tags Authentication
// @Produce json
// @Param debug query boolean false "Set to true to get authorization code for manual testing"
// @Success 200 {object} map[string]interface{} "Google OAuth URL"
// @Router /auth/google [get]
func GoogleLogin(c *gin.Context) {
	url := googleOauthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline)
	
	debugMode := c.Query("debug") == "true"
	
	instructions := "Open the authorization_url in your browser to sign in. You'll be automatically authenticated and receive a JWT token."
	if debugMode {
		instructions = "1. Open the authorization_url in your browser. 2. Sign in with Google. 3. You'll see a JSON response with a 'code' field. 4. Copy that code and use it with GET /auth/google/callback?code=YOUR_CODE to get your token."
		url = url + "&redirect_uri=" + config.AppConfig.GoogleCallbackURL + "?debug=true"
	}
	
	c.JSON(http.StatusOK, gin.H{
		"authorization_url": url,
		"debug_mode": debugMode,
		"instructions": instructions,
	})
}

// GoogleCallback godoc
// @Summary Google OAuth callback
// @Description This is automatically called by Google after sign-in. Add '?debug=true' to see the code without processing: /auth/google/callback?code=...&debug=true
// @Tags Authentication
// @Produce json
// @Param code query string true "Authorization code from Google"
// @Param debug query boolean false "Set to true to return code without processing"
// @Success 200 {object} map[string]interface{} "JWT token and user details (or code if debug=true)"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /auth/google/callback [get]
func GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code not provided"})
		return
	}

	if c.Query("debug") == "true" {
		c.JSON(http.StatusOK, gin.H{
			"code": code,
			"message": "Use this code with GET /auth/google/callback?code=YOUR_CODE (without debug parameter) to complete authentication",
		})
		return
	}

	token, err := googleOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		log.Println("Failed to exchange token:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token"})
		return
	}

	client := googleOauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		log.Println("Failed to get user info:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
		return
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	
	var googleUser struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	
	if err := json.Unmarshal(data, &googleUser); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse user info"})
		return
	}

	var user models.User
	result := database.DB.Where("google_id = ?", googleUser.ID).First(&user)
	
	if result.Error != nil {
		user = models.User{
			Email:    googleUser.Email,
			Name:     googleUser.Name,
			GoogleID: googleUser.ID,
		}
		
		if err := database.DB.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
			return
		}

		walletNumber, err := utils.GenerateWalletNumber()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate wallet number"})
			return
		}

		wallet := models.Wallet{
			UserID:       user.ID,
			WalletNumber: walletNumber,
			Balance:      0,
		}

		if err := database.DB.Create(&wallet).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create wallet"})
			return
		}
	}

	jwtToken, err := utils.GenerateJWT(user.ID, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	var wallet models.Wallet
	database.DB.Where("user_id = ?", user.ID).First(&wallet)

	c.JSON(http.StatusOK, gin.H{
		"token": jwtToken,
		"user": gin.H{
			"id":            user.ID,
			"email":         user.Email,
			"name":          user.Name,
			"wallet_number": wallet.WalletNumber,
		},
	})
}
