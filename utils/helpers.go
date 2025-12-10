package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"time"
)

// GenerateAPIKey generates a secure random API key
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "sk_live_" + base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateWalletNumber generates a unique 13-digit wallet number
func GenerateWalletNumber() (string, error) {
	// Generate a random 13-digit number
	max := big.NewInt(9999999999999)
	min := big.NewInt(1000000000000)
	
	n, err := rand.Int(rand.Reader, new(big.Int).Sub(max, min))
	if err != nil {
		return "", err
	}
	
	n.Add(n, min)
	return fmt.Sprintf("%013d", n), nil
}

// ParseExpiry converts expiry string (1H, 1D, 1M, 1Y) to time.Time
func ParseExpiry(expiry string) (time.Time, error) {
	now := time.Now()
	
	if len(expiry) < 2 {
		return time.Time{}, fmt.Errorf("invalid expiry format")
	}

	value := expiry[:len(expiry)-1]
	unit := expiry[len(expiry)-1:]

	var duration time.Duration
	switch unit {
	case "H":
		duration = time.Hour
	case "D":
		duration = 24 * time.Hour
	case "M":
		duration = 30 * 24 * time.Hour
	case "Y":
		duration = 365 * 24 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("invalid expiry unit: must be H, D, M, or Y")
	}

	// Parse the numeric value
	var count int
	_, err := fmt.Sscanf(value, "%d", &count)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid expiry value")
	}

	return now.Add(time.Duration(count) * duration), nil
}

// GenerateReference generates a unique transaction reference
func GenerateReference() string {
	return fmt.Sprintf("TXN_%d", time.Now().UnixNano())
}
