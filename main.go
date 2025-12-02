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
var swService *services.SplitwiseService // Add this

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

	// 3. Sync Logic (Sequential)
	fmt.Println("🔄 Starting Data Sync...")

	// A. Sync Bank
	if err := sfService.Sync(); err != nil {
		fmt.Println("⚠️ SimpleFIN warning:", err)
	}

	// B. Sync Splitwise
	if err := swService.Sync(); err != nil {
		fmt.Println("⚠️ Splitwise warning:", err)
	}

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

// ... handlers ...

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Expense Tracker Running"))
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	// Trigger both in parallel or sequence
	go func() {
		sfService.Sync()
		swService.Sync()
	}()
	w.Write([]byte("Sync Started in Background"))
}
