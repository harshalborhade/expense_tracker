package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"expense_tracker/database"

	"gorm.io/gorm"
)

type SplitwiseService struct {
	DB     *gorm.DB
	APIKey string
	UserID int // We need to know "Who am I?" to calculate shares
	Rules  *RuleEngine
}

func NewSplitwiseService(db *gorm.DB, apiKey string, rules *RuleEngine) *SplitwiseService {
	return &SplitwiseService{
		DB:     db,
		APIKey: apiKey,
		Rules:  rules,
	}
}

// --- JSON Structs for Parsing ---

type SWUserResp struct {
	User struct {
		ID int `json:"id"`
	} `json:"user"`
}

type SWExpensesResp struct {
	Expenses []SWExpense `json:"expenses"`
}

type SWExpense struct {
	ID          int      `json:"id"`
	Date        string   `json:"date"` // "2023-10-27T10:00:00Z"
	Description string   `json:"description"`
	Cost        string   `json:"cost"`
	Currency    string   `json:"currency_code"`
	DeletedAt   *string  `json:"deleted_at"` // Null if active
	Users       []SWUser `json:"users"`
}

type SWUser struct {
	UserID     int    `json:"user_id"`
	PaidShare  string `json:"paid_share"`
	OwedShare  string `json:"owed_share"`
	NetBalance string `json:"net_balance"`
}

// --- Methods ---

// 1. Get Current User ID (Required to know which share is mine)
func (s *SplitwiseService) GetMyID() error {
	if s.UserID != 0 {
		return nil // Already fetched
	}

	req, _ := http.NewRequest("GET", "https://secure.splitwise.com/api/v3.0/get_current_user", nil)
	req.Header.Add("Authorization", "Bearer "+s.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Splitwise Auth Error: %d", resp.StatusCode)
	}

	var data SWUserResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}
	s.UserID = data.User.ID
	fmt.Printf("👤 Logged in as Splitwise User ID: %d\n", s.UserID)
	return nil
}

// 2. Sync Expenses
func (s *SplitwiseService) Sync() error {
	if s.APIKey == "" {
		return nil
	}

	// FIX 1: Ensure the Splitwise "Account" exists in the map
	// This stops the "record not found" error during export
	s.ensureAccountExists("splitwise_group", "Splitwise Shared Expenses")

	if err := s.GetMyID(); err != nil {
		return err
	}

	// Fetch expenses (Limit 50 for now, ideally use updated_after for incremental)
	req, _ := http.NewRequest("GET", "https://secure.splitwise.com/api/v3.0/get_expenses?limit=50", nil)
	req.Header.Add("Authorization", "Bearer "+s.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var data SWExpensesResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	count := 0
	for _, exp := range data.Expenses {
		// Skip deleted expenses
		if exp.DeletedAt != nil {
			continue
		}

		// Find My Share logic
		var myShare float64
		var didIPay bool

		for _, u := range exp.Users {
			if u.UserID == s.UserID {
				// Parse strings to float
				owed, _ := strconv.ParseFloat(u.OwedShare, 64)
				paid, _ := strconv.ParseFloat(u.PaidShare, 64)

				// "My Share" is what I consumed (OwedShare)
				// Expense tracker usually tracks what you CONSUMED, regardless of who paid.
				// We store this as negative (Expense)
				if owed > 0 {
					myShare = -owed
				}

				if paid > 0 {
					didIPay = true
				}
			}
		}

		// If I wasn't involved (share is 0), skip
		if myShare == 0 {
			continue
		}

		// Parse Date (Splitwise format: 2023-10-27T14:47:06Z)
		parsedTime, _ := time.Parse(time.RFC3339, exp.Date)
		dateStr := parsedTime.Format("2006-01-02")

		// Create ID (Prefix with sw_ to avoid collision)
		txID := fmt.Sprintf("sw_%d", exp.ID)

		// Note on Category:
		// If I PAID, this transaction duplicates the Bank transaction.
		// If I DID NOT PAY, this is a fresh expense.
		// We flag this in the Provider or Notes for later logic.
		providerLabel := "splitwise"
		if didIPay {
			providerLabel = "splitwise_payer"
		}

		// Upsert
		var existing database.Transaction
		result := s.DB.Limit(1).Find(&existing, "id = ?", txID)

		if result.RowsAffected == 0 {

			cat := "Expenses:Uncategorized"

			if match := s.Rules.Apply(exp.Description); match != "" {
				cat = match
			}

			// Record doesn't exist -> Create it
			tx := database.Transaction{
				ID:             txID,
				Provider:       providerLabel,
				AccountID:      "splitwise_group",
				Date:           dateStr,
				Payee:          exp.Description,
				Amount:         myShare,
				Currency:       exp.Currency,
				LedgerCategory: cat,
				IsReviewed:     false,
			}
			s.DB.Create(&tx)
			count++
		} else {
			// Record exists -> Update it
			existing.Amount = myShare
			existing.Date = dateStr
			if !existing.IsReviewed {
				existing.Payee = exp.Description
			}
			s.DB.Save(&existing)
		}
	}

	fmt.Printf("✅ Synced %d Splitwise Expenses\n", count)
	return nil
}

func (s *SplitwiseService) ensureAccountExists(id, name string) {
	var count int64
	s.DB.Model(&database.AccountMap{}).Where("external_id = ?", id).Count(&count)
	if count == 0 {
		s.DB.Create(&database.AccountMap{
			ExternalID:    id,
			Provider:      "splitwise",
			Name:          name,
			LedgerAccount: "Liabilities:Payable:Splitwise", // Default
		})
	}
}
