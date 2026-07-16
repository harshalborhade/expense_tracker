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
	// AmountCents stores money as integer minor units (cents) to avoid
	// floating-point representation errors. Positive = inflow, negative = outflow.
	AmountCents int64
	Currency    string

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

	if err := migrateAmountToCents(db); err != nil {
		return nil, err
	}

	return db, nil
}

// migrateAmountToCents converts the legacy floating-point `amount` column into
// the integer `amount_cents` column exactly once. It is a no-op on fresh
// databases (which never had an `amount` column) and is idempotent on
// already-migrated databases, since rows that already have a non-zero
// amount_cents are left untouched.
func migrateAmountToCents(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&Transaction{}, "amount") {
		return nil
	}
	// Newly added columns are NULL for pre-existing rows, and NULL = 0 is false
	// in SQL, so we must match NULL explicitly. Rows already migrated have a
	// non-zero amount_cents and are skipped, keeping this idempotent.
	return db.Exec(
		"UPDATE transactions SET amount_cents = CAST(ROUND(amount * 100) AS INTEGER) " +
			"WHERE (amount_cents IS NULL OR amount_cents = 0) AND amount <> 0",
	).Error
}

// GetLedgerAccountName resolves an external account ID to its mapped Ledger
// account name, falling back to a FIXME placeholder when unmapped.
func GetLedgerAccountName(db *gorm.DB, externalID string) string {
	var mapping AccountMap
	result := db.First(&mapping, "external_id = ?", externalID)

	if result.Error == nil && mapping.LedgerAccount != "" {
		return mapping.LedgerAccount
	}
	return "Assets:FIXME:" + externalID
}
