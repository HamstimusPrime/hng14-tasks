# Authentication Flow — Insighta

Both the CLI tool and the web portal authenticate via **GitHub OAuth with PKCE (S256)**. They share the same backend endpoints but diverge in how tokens are delivered and stored.

---

## Shared Concepts

| Concept | Detail |
|---|---|
| **OAuth provider** | GitHub (`https://github.com/login/oauth/authorize`) |
| **PKCE method** | S256 — `code_challenge = BASE64URL(SHA256(code_verifier))` |
| **Access token** | HS256-signed JWT, valid for **3 minutes**, carries `user_id`, `username`, `role`, `is_active` |
| **Refresh token** | Opaque 64-char hex string; only its SHA-256 hash is stored in the DB; valid for **5 minutes** |
| **Roles** | `analyst` (read-only) and `admin` (read + write). New accounts default to `analyst`. |

---

## CLI Flow (`insighta login`)

The CLI runs entirely from the terminal. The user never types a password — the browser handles GitHub authentication.

```
User terminal          Local server           Backend (port 8080)      GitHub
     |                 (port 8085)                    |                   |
     |--insighta login-->|                            |                   |
     |                   |-- generate state, code_verifier, code_challenge
     |                   |-- bind :8085/callback                          |
     |                   |                            |                   |
     |<--open browser----|                            |                   |
     |                                                |                   |
     |      browser hits GET /auth/github?source=cli&state=…&code_challenge=…&redirect_uri=http://localhost:8085/callback
     |                                                |                   |
     |                                     302 redirect to github.com/login/oauth/authorize
     |                                                                    |
     |      user approves on github.com                                  |
     |                                                                    |
     |<--GET http://localhost:8085/callback?code=…&state=…---------------| (GitHub redirects back)
     |                   |                            |                   |
     |                   |-- validate state (CSRF check)                  |
     |                   |                            |                   |
     |                   |--POST /auth/github/callback (JSON: code, code_verifier, redirect_uri)
     |                   |                            |                   |
     |                   |          exchange code+verifier with GitHub    |
     |                   |                            |--POST githubTokenURL
     |                   |                            |<--access_token----|
     |                   |                            |                   |
     |                   |                  fetch GitHub user profile     |
     |                   |                            |--GET api.github.com/user
     |                   |                            |<--{ id, login, email, avatar_url }
     |                   |                            |
     |                   |                  upsert auth_users row (create or update)
     |                   |                  issue JWT + refresh token, store token hash in DB
     |                   |                            |
     |                   |<--200 { access_token, refresh_token, username, role }
     |                   |
     |    save tokens to ~/.insighta/tokens.json (mode 0600)
     |    print "Logged in as @username"
```

### Step-by-step

1. **Generate credentials** — CLI creates a random `state` (64-char hex) and `code_verifier` (64-char hex), then computes `code_challenge = BASE64URL(SHA256(code_verifier))`.
2. **Start local HTTP server** — binds `127.0.0.1:8085`; a single `/callback` handler waits for GitHub's redirect.
3. **Open browser** — launches the user's default browser pointing at `GET /auth/github?source=cli&state=…&code_challenge=…&redirect_uri=http://localhost:8085/callback`.
4. **Backend proxies to GitHub** — the server just passes the CLI-supplied params through to `https://github.com/login/oauth/authorize` (302 redirect). The backend does **not** store anything for the CLI flow.
5. **GitHub redirects back** — after the user approves, GitHub calls `http://localhost:8085/callback?code=…&state=…`.
6. **CSRF check** — CLI confirms the returned `state` matches what it generated; aborts on mismatch.
7. **Token exchange** — CLI POSTs `{ code, code_verifier, redirect_uri }` as JSON to `POST /auth/github/callback`. The backend exchanges these with GitHub (passing `code_verifier` to complete PKCE), fetches the user's GitHub profile, upserts the `auth_users` row, and returns `{ access_token, refresh_token, username, role }`.
8. **Store tokens** — CLI writes the token pair to `~/.insighta/tokens.json` with file permissions `0600`.

### Subsequent API calls

Every command (`profiles`, `profile`, `search`) calls `authedRequest`, which:
- Reads tokens from `~/.insighta/tokens.json`.
- Sends `Authorization: Bearer <access_token>` with the request.
- On **HTTP 401**, automatically posts `{ refresh_token }` to `POST /auth/refresh` to get a new token pair (rotation — the old refresh token is immediately revoked), saves the new tokens, and retries the original request once.
- On any other failure, prints an error and suggests running `insighta login`.

---

## Web Portal Flow

The browser user visits `/web/login` and clicks "Login with GitHub". No terminal is involved.

```
Browser                       Backend (port 8080)           GitHub
   |                                 |                          |
   |--GET /web/login---------------->|                          |
   |<--render login.html-------------|                          |
   |                                                            |
   |--click "Login with GitHub"                                 |
   |--GET /auth/github?source=web--->|                          |
   |                      generate state + code_verifier        |
   |                      store { verifier, createdAt } in memory (5-min TTL)
   |                                 |                          |
   |<--302 redirect to github.com/login/oauth/authorize-------->|
   |                                                            |
   |  user approves on github.com                              |
   |                                                            |
   |<--GET /auth/github/callback?code=…&state=…----------------|
   |                                 |                          |
   |              look up state in memory; verify not expired   |
   |              exchange code + stored verifier with GitHub   |
   |                                 |--POST githubTokenURL---->|
   |                                 |<--access_token-----------|
   |                                 |                          |
   |              fetch GitHub user profile                     |
   |                                 |--GET api.github.com/user>|
   |                                 |<--{ id, login, email }---|
   |                                 |
   |              upsert auth_users row
   |              issue JWT + refresh token, store token hash in DB
   |                                 |
   |<--302 /web/dashboard            |
   |    Set-Cookie: session=<JWT>; HttpOnly; SameSite=Lax; MaxAge=180
   |    Set-Cookie: refresh_token=<raw>; HttpOnly; Path=/auth; MaxAge=300
```

### Step-by-step

1. **Login page** — `GET /web/login` renders a static HTML page with a "Login with GitHub" button that links to `/auth/github?source=web`.
2. **Server generates PKCE** — on `GET /auth/github?source=web`, the backend generates `state` and `code_verifier`, computes `code_challenge`, stores `{ verifier, createdAt }` in an in-memory map keyed by `state`, then redirects the browser to GitHub.
3. **GitHub redirects back** — GitHub calls `GET /auth/github/callback?code=…&state=…` (the browser follows the redirect).
4. **State validation** — server looks up the `state` in the in-memory map. If missing or older than 5 minutes, the request is rejected.
5. **Token exchange** — same as CLI: backend sends `code + code_verifier` to GitHub, fetches the user profile, upserts `auth_users`.
6. **Set cookies** — two `HttpOnly` cookies are set:
   - `session` — the JWT access token (3-minute expiry, `MaxAge=180`).
   - `refresh_token` — the opaque refresh token (5-minute expiry, `MaxAge=300`, scoped to `Path=/auth`).
7. **Dashboard** — browser is redirected to `/web/dashboard`, which reads the `session` cookie, parses and validates the JWT, and renders the user's data.

### Session checks

- `GET /web/` — redirects to `/web/dashboard` if a valid `session` cookie is present, otherwise to `/web/login`.
- `GET /web/dashboard` — re-validates the JWT on every request; redirects to login on missing/expired token; returns 403 if the account is deactivated.

---

## Token Refresh

Both flows use the same endpoint for token rotation.

```
POST /auth/refresh
Body: { "refresh_token": "<raw token>" }

1. Hash the raw token (SHA-256).
2. Look up the hash in refresh_tokens table — reject if not found or expired.
3. Revoke (delete) the existing hash immediately.
4. Verify the linked user exists and is_active = true.
5. Issue a new JWT + refresh token pair (new hash written to DB).
6. Return: { access_token, refresh_token }
```

Refresh tokens are **single-use**: they are revoked at the moment they are consumed.

---

## Logout

```
POST /auth/logout
Body (optional): { "refresh_token": "<raw token>" }
```

- If a `refresh_token` is in the body, its hash is revoked in the DB.
- For web sessions, the `session` and `refresh_token` cookies are cleared by setting `MaxAge=-1`.
- The short-lived JWT itself is not blocklisted — it simply expires within 3 minutes.

---

## Role-Based Access Control

All API routes sit behind the `Authenticate` middleware, which reads the `Authorization: Bearer <JWT>` header. RBAC is then applied per route group:

| Route | Required role |
|---|---|
| `GET /api/profiles` | `analyst` or `admin` |
| `GET /api/profiles/search` | `analyst` or `admin` |
| `GET /api/profiles/:id` | `analyst` or `admin` |
| `POST /api/profiles` | `admin` only |
| `DELETE /api/profiles/:id` | `admin` only |

New GitHub users default to the `analyst` role. Role promotion to `admin` must be done directly in the database.

---

## Security Properties

| Property | How it's achieved |
|---|---|
| PKCE binding | `code_challenge` sent to GitHub; `code_verifier` sent back during token exchange — GitHub validates the pair, making stolen codes useless |
| CSRF protection | Random `state` matched before code is accepted |
| Refresh token security | Only SHA-256 hash stored in DB; raw token never persisted |
| Token rotation | Each refresh call revokes the old token before issuing a new one |
| Cookie isolation | `HttpOnly` prevents JS access; `refresh_token` scoped to `Path=/auth` |
| Inactive accounts | `is_active=false` blocks both cookie and Bearer token paths at the middleware level |
