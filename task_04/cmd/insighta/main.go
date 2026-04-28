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
		printCmdHelp()
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
		printCmdHelp()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cmdLogin() error {
	// 1. Generate state (CSRF token); verifier is generated server-side.
	//256 bit random value encoded to a 64char hex string
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// 2. Start local listener; Github server will redirect the browser here with tokens.
	const callbackPort = 8085
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		return fmt.Errorf("failed to start local server on port %d (check if another insighta login instance is already running?): %w", callbackPort, err)
	}

	type callbackResult struct {
		accessToken  string
		refreshToken string
		username     string
		err          error
		state        string
	}
	resultCh := make(chan callbackResult, 1)

	callBackMux := http.NewServeMux()
	callBackSrv := &http.Server{Handler: callBackMux}
	callBackMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			resultCh <- callbackResult{err: fmt.Errorf("github oauth error: %s", errParam)}
			fmt.Fprint(w, "<html><body>Authorization failed. You may close this tab.</body></html>")
			go callBackSrv.Shutdown(context.Background())
			return
		}
		resultCh <- callbackResult{
			accessToken:  r.URL.Query().Get("access_token"),
			refreshToken: r.URL.Query().Get("refresh_token"),
			username:     r.URL.Query().Get("username"),
			state:        r.URL.Query().Get("state"),
		}
		fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You may close this tab.</p></body></html>")
		go callBackSrv.Shutdown(context.Background())
	})
	go callBackSrv.Serve(listener)

	// 3. Build Github auth URL, include the
	// CSRF token(state) and make a request to
	// the Github URL in the browser.
	apiBase := getEnv("INSIGHTA_API", defaultBackend)
	authURL := fmt.Sprintf(
		"%s/auth/github?source=cli&state=%s&callback_port=%d",
		apiBase,
		url.QueryEscape(state),
		callbackPort,
	)
	fmt.Println("Opening browser for GitHub login...")
	openBrowser(authURL)

	// 4. Wait for the server redirect (2-minute timeout).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var res callbackResult
	//setup an async situation where 
	select {
	case res = <-resultCh:
	case <-ctx.Done():
		return fmt.Errorf("login timed out — no response after 2 minutes")
	}

	if res.err != nil {
		return res.err
	}
	if res.state != state {
		return fmt.Errorf("state mismatch: possible CSRF attack, aborting")
	}
	if res.accessToken == "" {
		return fmt.Errorf("no access token received from server")
	}

	if err := saveTokens(storedTokens{
		AccessToken:  res.accessToken,
		RefreshToken: res.refreshToken,
		Username:     res.username,
	}); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	fmt.Printf("Logged in as @%s\n", res.username)
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
	//use runtime.GOOS to get the native Operating
	//system the program is running on. in order
	//to select what command to launch browser
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

func printCmdHelp() {
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
