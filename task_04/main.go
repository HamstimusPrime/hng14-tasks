package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"hng_task_04/internal/database"
	"hng_task_04/internal/middleware"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	db                 *database.Queries
	jwtSecret          []byte
	githubClientID     string
	githubClientSecret string
	baseURL            string
}

type oauthSession struct {
	codeVerifier string
	createdAt    time.Time
}

// oauthStates holds in-flight web OAuth sessions keyed by state string.
var oauthStates = struct {
	sync.Mutex
	m map[string]oauthSession
}{m: make(map[string]oauthSession)}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading from environment")
	}

	db, err := sql.Open("postgres", mustEnv("DB_URL"))
	if err != nil {
		log.Fatalf("unable to establish connection to database: %v", err)
	}

	cfg := &apiConfig{
		db:                 database.New(db),
		jwtSecret:          []byte(mustEnv("JWT_SECRET")),
		githubClientID:     mustEnv("GITHUB_CLIENT_ID"),
		githubClientSecret: mustEnv("GITHUB_CLIENT_SECRET"),
		baseURL:            getEnv("BASE_URL", "http://localhost:8080"),
	}

	r := chi.NewRouter()

	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(9 * time.Second))

	// Public auth routes
	r.Get("/auth/github", cfg.handlerGitHubLogin)
	r.Get("/auth/github/callback", cfg.handlerWebCallback)
	r.Post("/auth/github/callback", cfg.handlerCLICallback)
	r.Post("/auth/refresh", cfg.handlerRefresh)
	r.Post("/auth/logout", cfg.handlerLogout)

	// Web portal routes
	r.Get("/web/", cfg.handlerWebRoot)
	r.Get("/web/login", cfg.handlerWebLogin)
	r.Get("/web/dashboard", cfg.handlerWebDashboard)
	r.Handle("/web/static/*", http.StripPrefix("/web/static/", http.FileServer(http.Dir("web/static"))))

	// Protected API routes — all require a valid JWT
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.jwtSecret))

		// analyst + admin: read-only
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("admin", "analyst"))
			r.Get("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
				handlerGetProfiles(w, r, cfg.db)
			})
			r.Get("/api/profiles/search", func(w http.ResponseWriter, r *http.Request) {
				handlerNLQsearch(w, r, cfg.db)
			})
			r.Get("/api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
				handlerGetProfileWithID(w, r, cfg.db)
			})
		})

		// admin only: write operations
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("admin"))
			r.Post("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
				handlerCreateProfile(w, r, cfg.db)
			})
			r.Delete("/api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
				handlerDeleteProfileWithID(w, r, cfg.db)
			})
		})
	})

	port := getEnv("PORT", "8080")
	fmt.Printf("Server starting on :%s\n", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
