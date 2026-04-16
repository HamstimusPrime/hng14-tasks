package main

import (
	"database/sql"
	"fmt"
	"hng_task_02/internal/database"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	DB_URL := os.Getenv("DB_URL")
	log.Fatalf("sql URL is %v", DB_URL)
	db, err := sql.Open("postgres", DB_URL)
	if err != nil {
		log.Fatalf("unable to establish connection to database: %v", err)
	}

	handler := http.TimeoutHandler(
		http.DefaultServeMux,
		9000*time.Millisecond,
		"request timed out",
	)

	queries := database.New(db)

	http.HandleFunc("POST /api/profiles", func(w http.ResponseWriter, r *http.Request) {
		handlerCreateProfile(w, r, queries)
	})
	http.HandleFunc("GET /api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		handlerGetProfileWithID(w, r, queries)
	})
	http.HandleFunc("GET /api/profiles", func(w http.ResponseWriter, r *http.Request) {
		handlerGetUsers(w, r, queries)
	})
	http.HandleFunc("DELETE /api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		handlerDeleteProfileWithID(w, r, queries)
	})

	// Start the server on the PORT environment variable or default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("Server starting on :%s...\n", port)

	http.ListenAndServe(":"+port, handler)
}
