package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Email     string    `gorm:"uniqueIndex;not null" json:"email"`
	Name      string    `json:"name"`
	GoogleID  string    `gorm:"uniqueIndex" json:"google_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Wallet      *Wallet       `gorm:"foreignKey:UserID" json:"wallet,omitempty"`
	APIKeys     []APIKey      `gorm:"foreignKey:UserID" json:"api_keys,omitempty"`
	Transactions []Transaction `gorm:"foreignKey:UserID" json:"transactions,omitempty"`
}

type Wallet struct {
	ID           string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID       string    `gorm:"uniqueIndex;not null" json:"user_id"`
	WalletNumber string    `gorm:"uniqueIndex;not null" json:"wallet_number"`
	Balance      int64     `gorm:"default:0" json:"balance"` // Store in kobo (smallest currency unit)
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

type TransactionType string
type TransactionStatus string

const (
	TransactionTypeDeposit  TransactionType = "deposit"
	TransactionTypeTransfer TransactionType = "transfer"
	TransactionTypeCredit   TransactionType = "credit" // When receiving transfer
)

const (
	TransactionStatusPending TransactionStatus = "pending"
	TransactionStatusSuccess TransactionStatus = "success"
	TransactionStatusFailed  TransactionStatus = "failed"
)

type Transaction struct {
	ID               string            `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID           string            `gorm:"not null;index" json:"user_id"`
	Type             TransactionType   `gorm:"not null" json:"type"`
	Amount           int64             `gorm:"not null" json:"amount"` // In kobo
	Status           TransactionStatus `gorm:"not null;default:'pending'" json:"status"`
	Reference        string            `gorm:"uniqueIndex" json:"reference"`
	RecipientWalletID *string          `json:"recipient_wallet_id,omitempty"`
	SenderWalletID    *string          `json:"sender_wallet_id,omitempty"`
	Metadata         string            `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`

	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

type APIKey struct {
	ID          string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID      string    `gorm:"not null;index" json:"user_id"`
	Name        string    `gorm:"not null" json:"name"`
	Key         string    `gorm:"uniqueIndex;not null" json:"key"`
	Permissions string    `gorm:"type:jsonb;not null" json:"permissions"` // Stored as JSON array
	ExpiresAt   time.Time `gorm:"not null" json:"expires_at"`
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (a *APIKey) IsExpired() bool {
	return time.Now().After(a.ExpiresAt)
}
