package services

import (
	"os"
	"path/filepath"
	"sort"
	"text/template"
	"time"

	"expense_tracker/database"

	"gorm.io/gorm"
)

type LedgerExportService struct {
	DB      *gorm.DB
	RootDir string
}

func NewLedgerExportService(db *gorm.DB, rootDir string) *LedgerExportService {
	if rootDir == "" {
		rootDir = "exports"
	}
	return &LedgerExportService{DB: db, RootDir: rootDir}
}

// Data structure for the template
type LedgerEntry struct {
	Date          string
	Payee         string
	Amount        float64
	Currency      string
	AccountDest   string
	AccountSource string
	Note          string
}

// Template for a single month file
const monthTemplate = `
; Expense Tracker - {{ .Month }}
; Auto-generated at {{ .GeneratedAt }}

{{ range .Entries }}
{{ .Date }} * {{ .Payee }}
    {{ .AccountDest }}      {{ printf "%.2f" .Amount }} {{ .Currency }}
    {{ .AccountSource }}
    {{ if .Note }}; {{ .Note }}{{ end }}
{{ end }}
`

// Template for the root main.journal
const mainIndexTemplate = `
; Main Index File
; Open this file with: ledger -f main.journal

; Includes by Year
{{ range .Years }}
include {{ . }}/{{ . }}*.journal
{{ end }}
`

func (s *LedgerExportService) Export() error {
	var transactions []database.Transaction

	// Fetch all transactions
	if err := s.DB.Order("date asc").Find(&transactions).Error; err != nil {
		return err
	}

	// 1. Bucketize by Year-Month (e.g. "2023-10")
	buckets := make(map[string][]LedgerEntry)
	years := make(map[string]bool) // Track unique years for the index file

	for _, tx := range transactions {
		// Determine Year and Month from Date string "YYYY-MM-DD"
		// Simple string slicing is safe because we enforce format in DB
		if len(tx.Date) < 7 {
			continue
		}
		year := tx.Date[0:4]
		monthKey := tx.Date[0:7] // "2023-10"

		years[year] = true

		// Ledger Logic (Same as before)
		sourceAcct := database.GetLedgerAccountName(s.DB, tx.AccountID, "Unknown")
		if tx.Provider == "splitwise" {
			sourceAcct = "Liabilities:Payable:Splitwise"
		} else if tx.Provider == "splitwise_payer" {
			continue // Skip reimbursement records to avoid duplicates
		}

		amount := tx.Amount * -1 // Flip sign

		entry := LedgerEntry{
			Date:          tx.Date,
			Payee:         tx.Payee,
			Amount:        amount,
			Currency:      tx.Currency,
			AccountDest:   tx.LedgerCategory,
			AccountSource: sourceAcct,
			Note:          tx.Notes,
		}

		buckets[monthKey] = append(buckets[monthKey], entry)
	}

	// 2. Write Month Files
	tmpl, err := template.New("ledger").Parse(monthTemplate)
	if err != nil {
		return err
	}

	for monthKey, entries := range buckets {
		// key: "2023-11" -> Year: "2023"
		year := monthKey[0:4]

		// Ensure directory exports/2023 exists
		yearDir := filepath.Join(s.RootDir, year)
		if err := os.MkdirAll(yearDir, 0755); err != nil {
			return err
		}

		// Create file exports/2023/2023-11.journal
		filePath := filepath.Join(yearDir, monthKey+".journal")
		f, err := os.Create(filePath)
		if err != nil {
			return err
		}

		data := struct {
			Month       string
			GeneratedAt string
			Entries     []LedgerEntry
		}{
			Month:       monthKey,
			GeneratedAt: time.Now().Format(time.RFC3339),
			Entries:     entries,
		}

		if err := tmpl.Execute(f, data); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}

	// 3. Write Main Index File (main.journal)
	return s.writeIndexFile(years)
}

func (s *LedgerExportService) writeIndexFile(yearsMap map[string]bool) error {
	// Sort years
	var years []string
	for y := range yearsMap {
		years = append(years, y)
	}
	sort.Strings(years)

	tmpl, err := template.New("index").Parse(mainIndexTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(s.RootDir, "main.journal"))
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, struct{ Years []string }{Years: years})
}
