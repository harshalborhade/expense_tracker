package database

import (
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const StorageDirPerms = 0755

// AccountMap links SimpleFIN Account IDs to Ledger Account Names
type AccountMap struct {
	ExternalID    string `gorm:"primaryKey"`
	Provider      string // "simplefin"
	Name          string
	LedgerAccount string
}

// Transaction represents a unified financial event
type Transaction struct {
	ID        string `gorm:"primaryKey"`
	Provider  string `gorm:"index"`
	AccountID string `gorm:"index"`

	Date     string
	Payee    string
	Amount   float64
	Currency string

	LedgerCategory string
	Notes          string
	IsReviewed     bool `gorm:"default:false"`
}

func InitDB(dbPath string) (*gorm.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, StorageDirPerms); err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&AccountMap{}, &Transaction{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
