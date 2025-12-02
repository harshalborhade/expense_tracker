package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"expense_tracker/database"

	"gorm.io/gorm"
)

type SimpleFinService struct {
	DB        *gorm.DB
	AccessURL string // Store the URL directly in the struct
}

func NewSimpleFinService(db *gorm.DB, accessURL string) *SimpleFinService {
	// Auto-correct URL: It must end in /accounts to get transaction data
	if accessURL != "" && !strings.HasSuffix(accessURL, "/accounts") {
		// Strip trailing slash if present
		accessURL = strings.TrimSuffix(accessURL, "/")
		accessURL = accessURL + "/accounts"
	}

	return &SimpleFinService{
		DB:        db,
		AccessURL: accessURL,
	}
}

// --- JSON Response Structures ---
type SFResponse struct {
	Errors   []string    `json:"errors"`
	Accounts []SFAccount `json:"accounts"`
}

type SFAccount struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Currency     string          `json:"currency"`
	Transactions []SFTransaction `json:"transactions"`
}

type SFTransaction struct {
	ID          string `json:"id"`
	Posted      int64  `json:"posted"`
	Amount      string `json:"amount"`
	Description string `json:"description"`
}

// Sync fetches data using the stored AccessURL
func (s *SimpleFinService) Sync() error {
	if s.AccessURL == "" {
		return errors.New("SIMPLEFIN_ACCESS_URL is missing in .env")
	}

	fmt.Printf("Debug: Fetching from %s...\n", s.AccessURL)

	resp, err := http.Get(s.AccessURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("API Error: %d", resp.StatusCode)
	}

	var sfResp SFResponse
	if err := json.NewDecoder(resp.Body).Decode(&sfResp); err != nil {
		return err
	}

	// Process Accounts & Transactions
	for _, acc := range sfResp.Accounts {
		s.ensureAccountExists(acc.ID, acc.Name)

		for _, t := range acc.Transactions {
			tm := time.Unix(t.Posted, 0)
			dateStr := tm.Format("2006-01-02")
			amt, _ := strconv.ParseFloat(t.Amount, 64)

			var existing database.Transaction
			err := s.DB.First(&existing, "id = ?", t.ID).Error

			if err == gorm.ErrRecordNotFound {
				// Create New
				tx := database.Transaction{
					ID:             t.ID,
					Provider:       "simplefin",
					AccountID:      acc.ID,
					Date:           dateStr,
					Payee:          t.Description,
					Amount:         amt,
					Currency:       acc.Currency,
					LedgerCategory: "Expenses:Uncategorized",
					IsReviewed:     false,
				}
				s.DB.Create(&tx)
			} else {
				// Update Existing
				existing.Amount = amt
				existing.Date = dateStr
				if !existing.IsReviewed {
					existing.Payee = t.Description
				}
				s.DB.Save(&existing)
			}
		}
	}
	fmt.Printf("✅ Synced %d Accounts via SimpleFIN\n", len(sfResp.Accounts))
	return nil
}

func (s *SimpleFinService) ensureAccountExists(id, name string) {
	var count int64
	s.DB.Model(&database.AccountMap{}).Where("external_id = ?", id).Count(&count)
	if count == 0 {
		s.DB.Create(&database.AccountMap{
			ExternalID:    id,
			Provider:      "simplefin",
			Name:          name,
			LedgerAccount: "Assets:FIXME:" + id,
		})
	}
}
