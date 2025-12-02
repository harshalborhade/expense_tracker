package database

import (
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// PlaidItem stores the connection credentials for a Bank
type PlaidItem struct {
	ID          uint   `gorm:"primaryKey"`
	ItemID      string `gorm:"unique"`
	AccessToken string
	NextCursor  string // Used for incremental syncing
	Institution string // e.g., "Chase"
}

// AccountMap links a Plaid/Splitwise ID to a Ledger Account Name
type AccountMap struct {
	ExternalID    string `gorm:"primaryKey"` // Plaid AccountID or Splitwise GroupID
	Name          string // Readable Name (e.g., "Chase Sapphire")
	Provider      string // "plaid" or "splitwise"
	LedgerAccount string // The output string (e.g., "Liabilities:US:Chase")
}

// Transaction represents a unified financial event
type Transaction struct {
	ID        string `gorm:"primaryKey"` // Plaid Transaction ID or "sw_{id}"
	Provider  string `gorm:"index"`      // "plaid" or "splitwise"
	AccountID string `gorm:"index"`      // Foreign Key to AccountMap.ExternalID

	Date     string
	Payee    string
	Amount   float64 // Negative = Outflow, Positive = Inflow
	Currency string

	// User Editable Fields
	LedgerCategory string // e.g., "Expenses:Food"
	Notes          string
	IsReviewed     bool `gorm:"default:false"`
}

// Permission 0755: Owner(rwx), Group(rx), Others(rx)
const StorageDirPerms = 0755

// InitDB sets up the SQLite connection
func InitDB(dbPath string) (*gorm.DB, error) {
	// 1. Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, StorageDirPerms); err != nil {
		return nil, err
	}

	// 2. Open DB
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&PlaidItem{}, &AccountMap{}, &Transaction{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
