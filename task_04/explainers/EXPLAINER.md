# How Insighta Labs+ Works — From Login to Dashboard

## What is this thing, anyway?

Insighta Labs+ is a **profile intelligence platform**. You give it a person's name, and it goes out to three different services on the internet to figure out statistical information about that name — the likely gender, the likely age, and the most probable country of origin. It stores all of that in a database and lets authorized users browse, search, and manage those profiles.

It has two faces: a **website** you open in a browser, and a **CLI** (Command Line Interface) — a program you type commands into in a terminal window, like a text-based remote control.

---

## The Two Entrances: Web and CLI

Think of the system like a building with two doors. One door has a big "Login with GitHub" button on a webpage. The other door is a command you type into your terminal: `insighta login`. Both doors lead to the same place — GitHub's login system — but they travel slightly different paths to get there.

---

## What is GitHub OAuth? (The Login System)

When you click "Login with GitHub" on a website, you're using something called **OAuth**. Here's the honest, non-obscured version of what's happening:

> The app doesn't know your GitHub password. Instead, it sends you to GitHub's own website and says: *"Hey GitHub, can you confirm this person is who they say they are?"* GitHub shows you a screen asking if you trust the app. If you click yes, GitHub sends the app a temporary secret code that proves you said yes.

That code is called an **authorization code**. The server then takes that code and secretly exchanges it with GitHub (using its own private credentials) to get a real **GitHub access token** — basically a VIP badge that lets the server ask GitHub "what's this user's name and email?"

---

## The PKCE Trick (Preventing Cheats)

There's a security trick layered on top of this called **PKCE** (which stands for something forgettable — the concept matters more). Here's the analogy:

Before starting the login process, the server quietly makes up two secret values:
- A **verifier** — like a secret word only it knows
- A **challenge** — a scrambled version of that secret word (using a one-way math function called SHA-256, like a blender you can't un-blend)

It sends the *scrambled* version to GitHub. Later, when exchanging the authorization code for a token, it also sends the *original* secret word. GitHub runs the scramble on it and checks: *does this produce the same scrambled version I saw earlier?* If yes — great, the right party is completing the login.

This means even if someone intercepts the authorization code mid-flight, they can't use it because they don't have the original secret word.

---

## The State Value (Stopping Impostors)

When the CLI starts a login, it generates a **state** — a 64-character random string of gibberish, like `a3f9d2...`. It puts this in the login URL. At the end of the login process, GitHub sends this same value back. The CLI checks: *is this the exact same gibberish I sent?* If not — abort. This stops a class of attack where a bad actor tricks you into completing *their* login flow instead of yours.

---

## Two Token Types: The VIP Pass and the Renewal Card

After login succeeds, the server creates **two tokens** and hands them to you:

**Access Token (3-minute VIP pass)**
This is a **JWT** — a JSON Web Token. Think of it as a self-contained ID card. It's a small blob of text that encodes your username, your role (more on this shortly), and an expiry time. The server "signs" it with a secret key, like a wax seal on a letter — anyone can read the card, but only the server could have sealed it. When you make a request to the API, you show this card. The server breaks the seal and reads it instantly — no database lookup needed.

It expires after **3 minutes** on purpose. Short-lived passes limit the damage if someone steals yours.

**Refresh Token (5-minute renewal card)**
Since your access pass expires so quickly, it would be annoying to use the app if you had to log in with GitHub every 3 minutes. So there's a second, longer-lived token: the refresh token. When your access token expires, you hand this "card of sort" to the server and say "give me a fresh access token." The server checks the database to make sure this card hasn't been used or cancelled, then hands you a brand-new pair of both tokens — and throws the old refresh token away (a concept known as  **Token rotation**).

---

## Where Tokens Are Stored

**In the CLI:** Tokens are saved in a file on your computer at `~/.insighta/tokens.json` (inside your home directory). The file looks like this:
```json
{
  "access_token": "eyJ...",
  "refresh_token": "a3f9d2...",
  "username": "yourname"
}
```
That `~` means your personal folder (like `/Users/yourname` on a Mac). The file permissions are set to `0600` — meaning only you, the owner, can read it. Nobody else on the same computer can peek at it.

**In the browser (Web portal):** Tokens are stored in **cookies** — small text snippets the browser saves and automatically attaches to every request. The session cookie (access token) is marked `HttpOnly`, meaning JavaScript running on the page cannot read it — only the browser sends it to your server when a request is made. This stops a whole category of attacks where malicious code tries to steal your session.

**In the database:** Refresh tokens are never stored in raw form. Instead, the server runs them through SHA-256 (that one-way blender again in order to encrypt it) and only stores the scrambled version. So even if someone broke into the database, they'd see a bunch of meaningless hashes — not the actual tokens.

---

## The Auto-Refresh Dance (CLI)

Every time you run a CLI command (like `insighta profiles`), the code does this:

1. Load the saved tokens from the file
2. Make the API request with the access token
3. If the server says "401 Unauthorized" (your pass expired), automatically call `/auth/refresh` with the refresh token
4. Save the brand-new token pair back to the file
5. Retry the original request — seamlessly

You as the user never notice. You just get your results.

---

## Roles: Admin vs. Analyst

When your account is created in the database, you get assigned a **role** — either `analyst` or `admin`. This is baked into your JWT, so the server knows it on every request.

- **Analysts** can read and search profiles — that's it
- **Admins** can do all of that, plus create new profiles and delete existing ones

This system is called **RBAC** (Role-Based Access Control). The server enforces this through layers it calls **middleware** — code that runs before your request reaches its destination, like a bouncer at each door checking your wristband before letting you through.

---

## The CLI Commands

The CLI program (`insighta`) accepts four commands:

| Command | What it does |
|---|---|
| `insighta login` | Opens your browser to GitHub login, waits for the result, saves your tokens |
| `insighta profiles` | Lists profiles from the database (with pagination) |
| `insighta profile <id>` | Gets one specific profile by its unique ID |
| `insighta search "young male from Nigeria"` | Natural language search — parses your words into database filters |

The login command is the clever one. It spins up a **tiny local web server** on your own computer (port 8085), just for a few seconds. After you approve the app on GitHub's website, GitHub redirects your browser to `http://localhost:8085/...?access_token=...`. The little local server catches that, reads the tokens from the URL, and then shuts itself down. It's like setting a trap door in your living room, having the tokens land in it, and then sealing the door again.

---

## What Happens When You Create a Profile

When an admin submits a name (say, "Mohammed"):

1. The server checks if that name already exists in the database — if yes, just return it
2. It calls **Genderize.io** → *"How likely is Mohammed to be male/female, and how confident are you?"*
3. It calls **Agify.io** → *"What age is most statistically associated with the name Mohammed?"*
4. It calls **Nationalize.io** → *"Which countries have the highest percentage of people named Mohammed?"*
5. It picks the top country by probability, determines an age group (child, teen, adult, senior), and saves everything to the database

---

## The Web Portal

The web portal is three HTML pages served by the same server:

- `/web/login` — just a card with a "Continue with GitHub" button
- `/web/` — automatically redirects you to login or dashboard depending on whether you have a valid session cookie
- `/web/dashboard` — shows a table of profiles; if you're an admin, you also see a form to create profiles and delete buttons on each row

The server uses **Go HTML templates** to build the pages — think of it like a letter template with `{{.Username}}` blanks that the server fills in with real values before sending the page to your browser.

---

## The Database

There are three tables:

**`users`** — the profile data (names, ages, genders, countries)

**`auth_users`** — one row per person who has logged in with GitHub, storing their GitHub ID, username, email, role, and whether they're active

**`refresh_tokens`** — every issued refresh token (as a hash), with an expiry time and a "revoked" flag. When you log out, the server marks your token as revoked. When you refresh, the old token is deleted and a new one is inserted.

---

## Putting It All Together

Here's the full journey when you type `insighta login`:

```
[You]           [CLI on your laptop]        [Our Server]          [GitHub]
  │                     │                        │                    │
  │  type: insighta     │                        │                    │
  │──────────────────→  │                        │                    │
  │                     │  generate random state │                    │
  │                     │  start local server    │                    │
  │                     │  ─────────────────────→                    │
  │                     │  GET /auth/github?state=abc&port=8085       │
  │                     │                        │  generate verifier │
  │                     │                        │  store state+verifier
  │  browser opens ←────│                        │  redirect to GitHub│
  │                     │                        │──────────────────→ │
  │  GitHub shows       │                        │                    │
  │  "Authorize?"       │                        │                    │
  │  [click YES]        │                        │                    │
  │                     │                        │    code + state  ← │
  │                     │                        │  exchange code     │
  │                     │                        │  verify PKCE       │
  │                     │                        │  create user in DB │
  │                     │                        │  mint JWT + refresh│
  │  browser redirects ←│────────────────────────│                    │
  │  to localhost:8085  │                        │                    │
  │  with tokens        │                        │                    │
  │                     │  catch tokens          │                    │
  │                     │  verify state matches  │                    │
  │                     │  save to ~/.insighta/  │                    │
  │  "Logged in as @you"│                        │                    │
  │ ←───────────────────│                        │                    │
```

Every subsequent command silently attaches your access token, auto-refreshes it when it expires, and talks to the database on your behalf — all without you ever seeing your password or worrying about whether your session is still valid.
