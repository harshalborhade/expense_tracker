package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"expense_tracker/database"
)

// DTOs for JSON responses
type TransactionDTO struct {
	ID             string  `json:"id"`
	Date           string  `json:"date"`
	Payee          string  `json:"payee"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	AccountName    string  `json:"account_name"`
	LedgerCategory string  `json:"category"`
	IsReviewed     bool    `json:"is_reviewed"`
	Note           string  `json:"note"`
}

// GET /api/transactions
func handleGetTransactions(w http.ResponseWriter, r *http.Request) {
	var txs []database.Transaction
	// Sort by date desc
	db.Order("date desc").Limit(500).Find(&txs)

	// Fetch accounts to resolve names
	var accounts []database.AccountMap
	db.Find(&accounts)
	acctMap := make(map[string]string)
	for _, a := range accounts {
		acctMap[a.ExternalID] = a.Name
	}

	var dtos []TransactionDTO
	for _, t := range txs {
		acctName := acctMap[t.AccountID]
		if acctName == "" {
			acctName = "Unknown"
		}

		dtos = append(dtos, TransactionDTO{
			ID:             t.ID,
			Date:           t.Date,
			Payee:          t.Payee,
			Amount:         t.Amount,
			Currency:       t.Currency,
			AccountName:    acctName,
			LedgerCategory: t.LedgerCategory,
			IsReviewed:     t.IsReviewed,
			Note:           t.Notes,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dtos)
}

// POST /api/transactions/update
func handleUpdateTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var payload struct {
		ID       string `json:"id"`
		Payee    string `json:"payee"`
		Category string `json:"category"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	var tx database.Transaction
	if err := db.First(&tx, "id = ?", payload.ID).Error; err != nil {
		http.Error(w, "Transaction not found", 404)
		return
	}

	// Update fields
	if payload.Payee != "" {
		tx.Payee = payload.Payee
	}
	if payload.Category != "" {
		tx.LedgerCategory = payload.Category
	}
	tx.Notes = payload.Note
	tx.IsReviewed = true

	db.Save(&tx)

	// Regenerate export immediately
	go exportService.Export()

	w.Write([]byte(`{"status":"ok"}`))
}

// GET /api/accounts
func handleGetAccounts(w http.ResponseWriter, r *http.Request) {
	var accounts []database.AccountMap
	db.Find(&accounts)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

// POST /api/accounts/update
func handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var payload struct {
		ID            string `json:"id"`
		LedgerAccount string `json:"ledger_account"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	var acc database.AccountMap
	if err := db.First(&acc, "external_id = ?", payload.ID).Error; err != nil {
		http.Error(w, "Account not found", 404)
		return
	}

	acc.LedgerAccount = strings.TrimSpace(payload.LedgerAccount)
	db.Save(&acc)

	// Regenerate export
	go exportService.Export()

	w.Write([]byte(`{"status":"ok"}`))
}

// GET /api/categories
func handleGetCategories(w http.ResponseWriter, r *http.Request) {
	var categories []string
	db.Model(&database.Transaction{}).Distinct("ledger_category").Pluck("ledger_category", &categories)
	sort.Strings(categories)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(categories)
}
