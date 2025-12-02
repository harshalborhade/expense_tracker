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
var exportService *services.LedgerExportService // Add this

func main() {
	godotenv.Load()

	// 1. DB
	var err error
	db, err = database.InitDB("data/ledger.db")
	if err != nil {
		log.Fatal("Database init failed:", err)
	}

	// 2. Services
	sfService = services.NewSimpleFinService(db, os.Getenv("SIMPLEFIN_ACCESS_TOKEN"))
	swService = services.NewSplitwiseService(db, os.Getenv("SPLITWISE_API_KEY"))

	// Export path from env or default
	exportPath := os.Getenv("LEDGER_FILE_PATH")
	exportService = services.NewLedgerExportService(db, exportPath)

	// 3. Initial Sync & Export
	runFullSync()

	// 4. Routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/sync", handleSync)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("🚀 Server running at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// Helper to run syncs then export
func runFullSync() {
	fmt.Println("🔄 Starting Data Sync...")

	if err := sfService.Sync(); err != nil {
		fmt.Println("⚠️ SimpleFIN warning:", err)
	}

	if err := swService.Sync(); err != nil {
		fmt.Println("⚠️ Splitwise warning:", err)
	}

	fmt.Println("💾 Generating Ledger File...")
	if err := exportService.Export(); err != nil {
		fmt.Println("❌ Export failed:", err)
	} else {
		fmt.Println("✅ Export Complete!")
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Expense Tracker Running"))
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	go runFullSync()
	w.Write([]byte("Sync Started"))
}
