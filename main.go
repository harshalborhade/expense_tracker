package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"expense_tracker/database"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

// Global DB instance
var db *gorm.DB

func main() {
	// 1. Load Environment Variables
	err := godotenv.Load()
	if err != nil {
		log.Println("Note: No .env file found, relying on system env variables")
	}

	// 2. Initialize Database
	err = nil
	db, err = database.InitDB("data/ledger.db")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✅ Database Initialized (ledger.db)")

	// 3. Basic Server Setup
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Test Route
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<h1>Ledger Hub is Running</h1>"))
	})

	fmt.Printf("🚀 Server running at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
