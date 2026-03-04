package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"expense_tracker/database"
	"expense_tracker/services"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

var db *gorm.DB
var sfService *services.SimpleFinService
var swService *services.SplitwiseService
var exportService *services.LedgerExportService
var ruleEngine *services.RuleEngine

func main() {
	// 1. Define Flags
	exportOnly := flag.Bool("export", false, "Regenerate Ledger files and exit")
	noSync := flag.Bool("no-sync", false, "Start server without running initial network sync")
	flag.Parse()

	// 2. Load Env & DB
	godotenv.Load()

	var err error
	db, err = database.InitDB("data/ledger.db")
	if err != nil {
		log.Fatal("Database init failed:", err)
	}

	// 3. Init Rule Engine
	ruleEngine = services.NewRuleEngine(db)
	seedDefaultRules(db)
	ruleEngine.Reload()

	// 4. Init Services
	sfService = services.NewSimpleFinService(db, os.Getenv("SIMPLEFIN_ACCESS_TOKEN"), ruleEngine)
	swService = services.NewSplitwiseService(db, os.Getenv("SPLITWISE_API_KEY"), ruleEngine)

	exportPath := os.Getenv("LEDGER_FILE_PATH")
	exportService = services.NewLedgerExportService(db, exportPath)

	// 5. Handle Flags
	if *exportOnly {
		fmt.Println("[INFO] Export mode selected.")
		if err := exportService.Export(); err != nil {
			log.Fatalf("[ERROR] Export failed: %v", err)
		}
		fmt.Println("[SUCCESS] Ledger files generated.")
		return // Exit immediately
	}

	if *noSync {
		fmt.Println("[INFO] Skipping network sync (--no-sync).")
		// We still regenerate the file from the local DB to ensure it matches
		go func() {
			fmt.Println("[INFO] Regenerating ledger files from local DB...")
			exportService.Export()
		}()
	} else {
		// Default behavior: Full Sync
		go runFullSync()
	}

	// 6. Routes
	http.Handle("/", http.FileServer(http.Dir("./static")))

	http.HandleFunc("/api/sync", handleSync)
	http.HandleFunc("/api/transactions", handleGetTransactions)
	http.HandleFunc("/api/transactions/update", handleUpdateTransaction)
	http.HandleFunc("/api/accounts", handleGetAccounts)
	http.HandleFunc("/api/accounts/update", handleUpdateAccount)
	http.HandleFunc("/api/categories", handleGetCategories)

	http.HandleFunc("/api/rules", handleGetRules)
	http.HandleFunc("/api/rules/apply", handleApplyRules)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("[INFO] Server running at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func runFullSync() {
	fmt.Println("[INFO] Starting Data Sync...")

	if err := sfService.Sync(); err != nil {
		fmt.Printf("[WARN] SimpleFIN Error: %v\n", err)
	}

	if err := swService.Sync(); err != nil {
		fmt.Printf("[WARN] Splitwise Error: %v\n", err)
	}

	fmt.Println("[INFO] Generating Ledger File...")
	if err := exportService.Export(); err != nil {
		fmt.Printf("[ERROR] Export Failed: %v\n", err)
	} else {
		fmt.Println("[SUCCESS] Export Complete!")
	}
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	go runFullSync()
	w.Write([]byte(`{"status":"sync_started"}`))
}

func seedDefaultRules(db *gorm.DB) {
	var count int64
	db.Model(&database.CategoryRule{}).Count(&count)
	if count == 0 {
		fmt.Println("[INFO] Seeding default rules...")
		rules := []database.CategoryRule{
			{Priority: 10, Pattern: "(?i)uber|lyft", Category: "Expenses:Transport:Rideshare"},
			// {Priority: 10, Pattern: "(?i)starbucks|peet's", Category: "Expenses:Food:Coffee"},
			// {Priority: 10, Pattern: "(?i)safeway|trader joe", Category: "Expenses:Food:Groceries"},
			// {Priority: 10, Pattern: "(?i)netflix|spotify", Category: "Expenses:Entertainment:Subscriptions"},
			{Priority: 50, Pattern: "(?i)salary|payroll", Category: "Income:Salary"}, // High priority
		}
		db.Create(&rules)
	}
}
