package database

import (
	"os"
	"path/filepath"
	"time" // Added time

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const StorageDirPerms = 0755

// AccountMap links External IDs to Ledger Names AND stores Balances
type AccountMap struct {
	ExternalID    string `gorm:"primaryKey"`
	Provider      string // "simplefin" or "splitwise"
	Name          string
	LedgerAccount string

	// New Balance Fields
	CurrentBalance   float64
	AvailableBalance float64
	Currency         string
	LastUpdated      time.Time
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

// CategoryRule defines an automatic tagging rule
type CategoryRule struct {
	ID       uint   `gorm:"primaryKey"`
	Priority int    `gorm:"default:10"` // Higher number = runs first
	Pattern  string `gorm:"unique"`     // Regex string (e.g. "(?i)uber")
	Category string // The target category (e.g. "Expenses:Transport")
}

// InitDB initializes the database and performs migrations
func InitDB(dbPath string) (*gorm.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, StorageDirPerms); err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&AccountMap{}, &Transaction{}, &CategoryRule{})
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Helper to look up account mapping
func GetLedgerAccountName(db *gorm.DB, externalID, defaultName string) string {
	var mapping AccountMap
	result := db.First(&mapping, "external_id = ?", externalID)

	if result.Error == nil && mapping.LedgerAccount != "" {
		return mapping.LedgerAccount
	}
	return "Assets:FIXME:" + externalID
}
