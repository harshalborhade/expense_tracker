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
	UserID int
	Rules  *RuleEngine
}

func NewSplitwiseService(db *gorm.DB, apiKey string, rules *RuleEngine) *SplitwiseService {
	return &SplitwiseService{
		DB:     db,
		APIKey: apiKey,
		Rules:  rules,
	}
}

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
	Date        string   `json:"date"`
	Description string   `json:"description"`
	Cost        string   `json:"cost"`
	Currency    string   `json:"currency_code"`
	Payment     bool     `json:"payment"` // Added this field
	DeletedAt   *string  `json:"deleted_at"`
	Users       []SWUser `json:"users"`
}

type SWUser struct {
	UserID     int    `json:"user_id"`
	PaidShare  string `json:"paid_share"`
	OwedShare  string `json:"owed_share"`
	NetBalance string `json:"net_balance"`
}

func (s *SplitwiseService) GetMyID() error {
	if s.UserID != 0 {
		return nil
	}
	req, _ := http.NewRequest("GET", "https://secure.splitwise.com/api/v3.0/get_current_user", nil)
	req.Header.Add("Authorization", "Bearer "+s.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("splitwise auth error: %d", resp.StatusCode)
	}

	var data SWUserResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}
	s.UserID = data.User.ID
	fmt.Printf("[INFO] Logged in as Splitwise User ID: %d\n", s.UserID)
	return nil
}

func (s *SplitwiseService) Sync() error {
	if s.APIKey == "" {
		return nil
	}

	s.ensureAccountExists("splitwise_group", "Splitwise Shared Expenses")

	if err := s.GetMyID(); err != nil {
		return err
	}

	// Fetch recent expenses (limit 50 is usually enough for daily syncs)
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
		if exp.DeletedAt != nil {
			continue
		}

		// --- NEW LOGIC: Handle Payments vs Expenses ---
		var myAmount float64
		var didIPay bool
		involved := false

		for _, u := range exp.Users {
			if u.UserID == s.UserID {
				owed, _ := strconv.ParseFloat(u.OwedShare, 64)
				paid, _ := strconv.ParseFloat(u.PaidShare, 64)

				if exp.Payment {
					// Settlement Logic
					if paid > 0 {
						// I Paid (Settling debt) -> Positive Amount (reduces liability)
						myAmount = paid
						involved = true
					} else if owed > 0 {
						// I Received (Others settling debt to me) -> Negative Amount (reduces asset)
						myAmount = -owed
						involved = true
					}
				} else {
					// Expense Logic
					if owed > 0 {
						// I owe money -> Negative (increases liability)
						myAmount = -owed
						involved = true
					}
					if paid > 0 {
						didIPay = true
					}
				}
			}
		}

		// Skip if I'm not involved or the amount is effectively 0
		if !involved || myAmount == 0 {
			continue
		}

		parsedTime, _ := time.Parse(time.RFC3339, exp.Date)
		dateStr := parsedTime.Format("2006-01-02")
		txID := fmt.Sprintf("sw_%d", exp.ID)

		providerLabel := "splitwise"
		// If I paid for a group expense (reimbursement), mark it special
		// But if it's a direct payment (settlement), keep it standard "splitwise" so it shows up in main lists easily
		if didIPay && !exp.Payment {
			providerLabel = "splitwise_payer"
		} else if exp.Payment {
			providerLabel = "splitwise_payment"
		}

		// Check existence
		var existing database.Transaction
		result := s.DB.Limit(1).Find(&existing, "id = ?", txID)

		if result.RowsAffected == 0 {
			// Determine Category
			cat := "Expenses:Uncategorized"

			if exp.Payment {
				// Force settlements to Transfer category
				cat = "Transfers:Splitwise"
			} else {
				// Run Auto-Rules for normal expenses
				if match := s.Rules.Apply(exp.Description); match != "" {
					cat = match
				}
			}

			tx := database.Transaction{
				ID:             txID,
				Provider:       providerLabel,
				AccountID:      "splitwise_group",
				Date:           dateStr,
				Payee:          exp.Description,
				Amount:         myAmount,
				Currency:       exp.Currency,
				LedgerCategory: cat,
				Notes:          "Sync Import",
				IsReviewed:     exp.Payment, // Auto-mark payments as reviewed since we know they are transfers
			}
			s.DB.Create(&tx)
			count++
		} else {
			// Update existing (e.g. if amount changed in Splitwise)
			// We generally trust Splitwise updates
			existing.Amount = myAmount
			existing.Date = dateStr
			if !existing.IsReviewed {
				existing.Payee = exp.Description
			}
			s.DB.Save(&existing)
		}
	}

	if count > 0 {
		fmt.Printf("[INFO] Synced %d new Splitwise items\n", count)
	}
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
			LedgerAccount: "Liabilities:Payable:Splitwise",
		})
	}
}
