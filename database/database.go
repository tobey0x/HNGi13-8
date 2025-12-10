package database

import (
	"log"
	"wallet-service/config"
	"wallet-service/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect() {
	var err error
	
	DB, err = gorm.Open(postgres.Open(config.AppConfig.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	log.Println("Database connected successfully")
}

func Migrate() {
	err := DB.AutoMigrate(
		&models.User{},
		&models.Wallet{},
		&models.Transaction{},
		&models.APIKey{},
	)
	
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	log.Println("Database migration completed")
}
