package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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

func GoogleLogin(c *gin.Context) {
	url := googleOauthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code not provided"})
		return
	}

	// Exchange code for token
	token, err := googleOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		log.Println("Failed to exchange token:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token"})
		return
	}

	// Get user info from Google
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

	// Find or create user
	var user models.User
	result := database.DB.Where("google_id = ?", googleUser.ID).First(&user)
	
	if result.Error != nil {
		// Create new user
		user = models.User{
			Email:    googleUser.Email,
			Name:     googleUser.Name,
			GoogleID: googleUser.ID,
		}
		
		if err := database.DB.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
			return
		}

		// Create wallet for new user
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

	// Generate JWT
	jwtToken, err := utils.GenerateJWT(user.ID, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Redirect to frontend with token
	redirectURL := fmt.Sprintf("%s/auth/callback?token=%s", config.AppConfig.FrontendURL, jwtToken)
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}
