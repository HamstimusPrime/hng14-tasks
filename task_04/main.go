package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
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
	rawDB              *sql.DB
	jwtSecret          []byte
	githubClientID     string
	githubClientSecret string
	baseURL            string
}

type oauthSession struct {
	codeVerifier string
	createdAt    time.Time
	source       string // "web" or "cli"
	callbackPort string // only set for CLI flow
}

// oauthStates holds in-flight web OAuth sessions keyed by state string.
var oauthStates = struct {
	sync.Mutex
	m map[string]oauthSession
}{m: make(map[string]oauthSession)}

// corsMiddleware sets permissive CORS headers for all routes.
func corsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ipRateLimiter tracks per-IP request timestamps for sliding-window rate limiting.
type ipRateLimiter struct {
	sync.Mutex
	requests map[string][]time.Time
}

func newIPRateLimiter() *ipRateLimiter {
	return &ipRateLimiter{requests: make(map[string][]time.Time)}
}

func (l *ipRateLimiter) allow(ip string, limit int, window time.Duration) bool {
	l.Lock()
	defer l.Unlock()
	now := time.Now()
	cutoff := now.Add(-window)
	var recent []time.Time
	for _, t := range l.requests[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= limit {
		l.requests[ip] = recent
		return false
	}
	l.requests[ip] = append(recent, now)
	return true
}

func rateLimitMiddleware(limiter *ipRateLimiter, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !limiter.allow(ip, limit, window) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"status":"error","message":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

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
		rawDB:              db,
		jwtSecret:          []byte(mustEnv("JWT_SECRET")),
		githubClientID:     mustEnv("GITHUB_CLIENT_ID"),
		githubClientSecret: mustEnv("GITHUB_CLIENT_SECRET"),
		baseURL:            getEnv("BASE_URL", "http://localhost:8080"),
	}

	authLimiter := newIPRateLimiter()

	r := chi.NewRouter()

	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(9 * time.Second))
	r.Use(corsMiddleware())

	// Public auth routes
	r.With(rateLimitMiddleware(authLimiter, 10, time.Minute)).Get("/auth/github", cfg.handlerGitHubLogin)
	r.Get("/auth/github/callback", cfg.handlerWebCallback)
	r.Post("/auth/refresh", cfg.handlerRefresh)
	r.Post("/auth/logout", cfg.handlerLogout)
	r.Post("/auth/test/token", cfg.handlerTestToken)

	// Web portal routes
	r.Get("/web/", cfg.handlerWebRoot)
	r.Get("/web/login", cfg.handlerWebLogin)
	r.Get("/web/dashboard", cfg.handlerWebDashboard)
	r.Handle("/web/static/*", http.StripPrefix("/web/static/", http.FileServer(http.Dir("web/static"))))

	// Protected API routes — all require a valid JWT.
	// Registered under both /api/ (legacy) and /api/v1/ (versioned).
	registerAPIRoutes := func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.jwtSecret))

		r.Get("/users/me", cfg.handlerGetCurrentUser)

		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("admin", "analyst"))
			r.Get("/profiles", func(w http.ResponseWriter, r *http.Request) {
				handlerGetProfiles(w, r, cfg.db)
			})
			r.Get("/profiles/search", func(w http.ResponseWriter, r *http.Request) {
				handlerNLQsearch(w, r, cfg.db)
			})
			r.Get("/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
				handlerGetProfileWithID(w, r, cfg.db)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("admin"))
			r.Post("/profiles", func(w http.ResponseWriter, r *http.Request) {
				handlerCreateProfile(w, r, cfg.db)
			})
			r.Delete("/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
				handlerDeleteProfileWithID(w, r, cfg.db)
			})
		})
	}

	r.Route("/api", registerAPIRoutes)
	r.Route("/api/v1", registerAPIRoutes)

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
