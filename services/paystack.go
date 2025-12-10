package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"wallet-service/config"
)

type PaystackService struct{}

type InitializeTransactionRequest struct {
	Email     string `json:"email"`
	Amount    int64  `json:"amount"` // In kobo
	Reference string `json:"reference"`
}

type InitializeTransactionResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	} `json:"data"`
}

type VerifyTransactionResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Status    string `json:"status"`
	} `json:"data"`
}

func NewPaystackService() *PaystackService {
	return &PaystackService{}
}

func (ps *PaystackService) InitializeTransaction(email string, amount int64, reference string) (*InitializeTransactionResponse, error) {
	url := "https://api.paystack.co/transaction/initialize"

	payload := InitializeTransactionRequest{
		Email:     email,
		Amount:    amount,
		Reference: reference,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.AppConfig.PaystackSecretKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result InitializeTransactionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if !result.Status {
		return nil, fmt.Errorf("paystack error: %s", result.Message)
	}

	return &result, nil
}

func (ps *PaystackService) VerifyTransaction(reference string) (*VerifyTransactionResponse, error) {
	url := fmt.Sprintf("https://api.paystack.co/transaction/verify/%s", reference)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.AppConfig.PaystackSecretKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result VerifyTransactionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if !result.Status {
		return nil, fmt.Errorf("paystack error: %s", result.Message)
	}

	return &result, nil
}
