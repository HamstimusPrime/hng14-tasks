package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"hng_task_04/internal/auth"
)

const (
	defaultBackend = "http://localhost:8080"
	tokenFilePath  = ".insighta/tokens.json"
)

type storedTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Username     string `json:"username"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "login":
		err = cmdLogin()
	case "profiles":
		err = cmdProfiles(os.Args[2:])
	case "profile":
		err = cmdProfile(os.Args[2:])
	case "search":
		err = cmdSearch(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cmdLogin() error {
	// 1. Generate state and code_verifier (32 random bytes each → 64-char hex strings)
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	codeVerifier, err := auth.GenerateRandomHex(32)
	if err != nil {
		return fmt.Errorf("failed to generate code_verifier: %w", err)
	}

	// 2. Derive code_challenge
	codeChallenge := auth.CodeChallenge(codeVerifier)

	// 3. Start a local callback server on a fixed port so it matches the
	//    registered callback URL in the GitHub OAuth app settings.
	const callbackPort = 8085
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		return fmt.Errorf("failed to start local server on port %d (is another insighta login already running?): %w", callbackPort, err)
	}
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", callbackPort)

	codeCh := make(chan string, 1)
	stateCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			errCh <- fmt.Errorf("github oauth error: %s", errParam)
			fmt.Fprint(w, "<html><body>Authorization failed. You may close this tab.</body></html>")
			go srv.Shutdown(context.Background())
			return
		}
		codeCh <- r.URL.Query().Get("code")
		stateCh <- r.URL.Query().Get("state")
		fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You may close this tab.</p></body></html>")
		go srv.Shutdown(context.Background())
	})
	go srv.Serve(listener)

	// 4. Build the authorization URL via the backend (which proxies to GitHub)
	apiBase := getEnv("INSIGHTA_API", defaultBackend)
	authURL := fmt.Sprintf(
		"%s/auth/github?source=cli&state=%s&code_challenge=%s&code_challenge_method=S256&redirect_uri=%s",
		apiBase,
		url.QueryEscape(state),
		url.QueryEscape(codeChallenge),
		url.QueryEscape(redirectURI),
	)

	// 5. Open the browser
	fmt.Println("Opening browser for GitHub login...")
	openBrowser(authURL)

	// 6. Wait for the callback (2-minute timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var receivedCode, receivedState string
	select {
	case receivedCode = <-codeCh:
		receivedState = <-stateCh
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("login timed out — no response from GitHub after 2 minutes")
	}

	// 7. Validate state to prevent CSRF
	if receivedState != state {
		return fmt.Errorf("state mismatch: possible CSRF attack, aborting")
	}

	// 8. POST code + code_verifier to backend
	payload, _ := json.Marshal(map[string]string{
		"code":          receivedCode,
		"code_verifier": codeVerifier,
		"redirect_uri":  redirectURI,
	})
	resp, err := http.Post(apiBase+"/auth/github/callback", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to exchange code with backend: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backend returned HTTP %d during token exchange", resp.StatusCode)
	}

	// 9. Parse and store tokens
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Username     string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}
	if err := saveTokens(storedTokens{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		Username:     tokens.Username,
	}); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	fmt.Printf("Logged in as @%s\n", tokens.Username)
	return nil
}

func cmdProfiles(args []string) error {
	page, limit := "1", "10"
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--page":
			page = args[i+1]
		case "--limit":
			limit = args[i+1]
		}
	}
	resp, err := authedRequest(http.MethodGet, fmt.Sprintf("/api/profiles?page=%s&limit=%s", page, limit), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printJSON(resp)
}

func cmdProfile(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: insighta profile <id>")
	}
	resp, err := authedRequest(http.MethodGet, "/api/profiles/"+args[0], nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printJSON(resp)
}

func cmdSearch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: insighta search \"<query>\"")
	}
	q := url.QueryEscape(strings.Join(args, " "))
	resp, err := authedRequest(http.MethodGet, "/api/profiles/search?q="+q, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printJSON(resp)
}

// authedRequest sends an authenticated HTTP request.
// On 401, it attempts a single token refresh before retrying.
func authedRequest(method, path string, body []byte) (*http.Response, error) {
	tokens, err := loadTokens()
	if err != nil {
		return nil, err
	}
	apiBase := getEnv("INSIGHTA_API", defaultBackend)

	do := func(accessToken string) (*http.Response, error) {
		var bodyReader *bytes.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		} else {
			bodyReader = bytes.NewReader(nil)
		}
		req, err := http.NewRequest(method, apiBase+path, bodyReader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return http.DefaultClient.Do(req)
	}

	resp, err := do(tokens.AccessToken)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	resp.Body.Close()

	// Attempt token refresh
	refreshPayload, _ := json.Marshal(map[string]string{"refresh_token": tokens.RefreshToken})
	rr, err := http.Post(apiBase+"/auth/refresh", "application/json", bytes.NewReader(refreshPayload))
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w — run: insighta login", err)
	}
	defer rr.Body.Close()
	if rr.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("session expired (HTTP %d) — run: insighta login", rr.StatusCode)
	}

	var newTokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(rr.Body).Decode(&newTokens)
	saveTokens(storedTokens{
		AccessToken:  newTokens.AccessToken,
		RefreshToken: newTokens.RefreshToken,
		Username:     tokens.Username,
	})

	return do(newTokens.AccessToken)
}

// --- token file helpers ---

func tokenFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, tokenFilePath), nil
}

func saveTokens(t storedTokens) error {
	path, err := tokenFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(t, "", "  ")
	return os.WriteFile(path, data, 0600)
}

func loadTokens() (*storedTokens, error) {
	path, err := tokenFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("not logged in — run: insighta login")
	}
	var t storedTokens
	return &t, json.Unmarshal(data, &t)
}

// --- helpers ---

func printJSON(resp *http.Response) error {
	var v interface{}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func openBrowser(u string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		u = "url.dll,FileProtocolHandler " + u
	default:
		cmd = "xdg-open"
	}
	exec.Command(cmd, u).Start()
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func printUsage() {
	fmt.Print(`insighta — Insighta Labs+ CLI

Usage:
  insighta login                           Authenticate via GitHub OAuth
  insighta profiles [--page N --limit N]   List profiles (paginated)
  insighta profile <id>                    Get a single profile by UUID
  insighta search "<query>"               Natural language search

Environment:
  INSIGHTA_API   Backend URL (default: http://localhost:8080)
`)
}
