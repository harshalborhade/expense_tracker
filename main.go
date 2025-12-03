package main

import (
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

func main() {
	godotenv.Load()

	// 1. Init DB
	var err error
	db, err = database.InitDB("data/ledger.db")
	if err != nil {
		log.Fatal("Database init failed:", err)
	}

	// 2. Init Services
	sfService = services.NewSimpleFinService(db, os.Getenv("SIMPLEFIN_ACCESS_TOKEN"))
	swService = services.NewSplitwiseService(db, os.Getenv("SPLITWISE_API_KEY"))

	exportPath := os.Getenv("LEDGER_FILE_PATH")
	exportService = services.NewLedgerExportService(db, exportPath)

	// 3. Run Sync on Startup
	go runFullSync()

	// 4. Routes

	// Serve the UI (Static Files)
	http.Handle("/", http.FileServer(http.Dir("./static")))

	// API Endpoints (Required for UI to work)
	http.HandleFunc("/api/sync", handleSync)
	http.HandleFunc("/api/transactions", handleGetTransactions)
	http.HandleFunc("/api/transactions/update", handleUpdateTransaction)
	http.HandleFunc("/api/accounts", handleGetAccounts)
	http.HandleFunc("/api/accounts/update", handleUpdateAccount)
	http.HandleFunc("/api/categories", handleGetCategories)

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
