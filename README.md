# Hackerthon

A competitive coding platform where developers solve algorithmic challenges, compete in timed tournaments, fight in clan raids, and challenge each other head-to-head. Built with Go on the backend and vanilla JS on the frontend.

---

## Table of Contents

- [Features](#features)
- [Tech Stack](#tech-stack)
- [Project Structure](#project-structure)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [Core Concepts](#core-concepts)
- [API Reference](#api-reference)
- [Frontend Architecture](#frontend-architecture)
- [Admin Panel](#admin-panel)
- [Deployment](#deployment)

---

## Features

### For Users
- **Questions** — Browse and solve coding challenges with an in-browser CodeMirror editor (syntax highlighting, autocomplete, bracket matching) across Go, Python, JavaScript, TypeScript, Java, C, C++, Rust, and Bash
- **Code Runner** — Run code against test cases instantly; see per-case pass/fail output before submitting
- **AI Hints** — Request AI-generated nudges when stuck (rate-limited per user per question)
- **Hints & Docs** — Static hints and reference doc links on every question
- **Submissions** — Review your past solutions and see community submissions with ratings
- **Progress Tracking** — Track solved questions by difficulty; mark questions complete
- **Leaderboard** — Global rankings by questions solved, average rating, and reviews given
- **Social** — Follow other users; get a feed of their recent solves
- **Search** — Find users by username

### Competitive Modes
- **Challenges** — 1v1 async challenges; send a challenge request to any user, accept/reject, then compete on a shared question set within a time limit
- **Tournaments** — Multi-player timed arenas; join upcoming tournaments, enter the arena when live, solve questions to earn points on a live leaderboard
- **Clan Raids** — Team-based coding battles; create or join a clan, participate in raids where clan members' solves contribute to a shared clan score shown on a live scoreboard

### Contest Features
- **Anti-cheat** — Tab-switch and copy/paste detection during active contest sessions
- **Contest Timer** — Countdown bar shared across challenges and tournaments; auto-exits arena on time expiry
- **Live Scoreboard** — Clan raid scoreboard polls every 5 seconds; tournament leaderboard available post-contest

### Admin
- Full question CRUD (title, description, difficulty, category, hint, hint URL, test cases, optional Go test file, visibility toggle)
- User management (view all users, promote/demote admin)
- Challenge and tournament creation; assign question sets and durations
- Raid creation and management

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go (standard library + `github.com/mattn/go-sqlite3`) |
| Database | SQLite (single file, zero-config) |
| Frontend | Vanilla JS, HTML, CSS (no framework) |
| Editor | CodeMirror 5.65 (Dracula theme) |
| Deployment | Render (via `render.yaml`) |

---

## Project Structure

```
hackerthon/
├── go/
│   ├── server.go               # HTTP server bootstrap, middleware
│   ├── router.go               # Route registration
│   ├── db.go                   # SQLite connection, schema migrations
│   ├── model.go                # Shared data types / structs
│   ├── auth.go                 # JWT issuance and validation
│   ├── runner.go               # Code execution sandbox (Docker or subprocess)
│   ├── runner_parser.go        # Parse runner stdout into test results
│   ├── challenge_engine.go     # Challenge lifecycle (create, enter, submit)
│   ├── handlers_auth.go        # /api/register, /api/login, /api/me
│   ├── handlers_questions.go   # /api/questions CRUD + visibility
│   ├── handlers_submissions.go # /api/submissions, run + submit endpoints
│   ├── handlers_challenge.go   # /api/challenges (1v1 challenges)
│   ├── handlers_tournament.go  # /api/tournaments
│   ├── handlers_raid.go        # /api/raids (clan raids)
│   ├── handlers_clan.go        # /api/clans
│   ├── handlers_social.go      # /api/follow, /api/feed, /api/search
│   ├── handlers_hint.go        # /api/hints, /api/ai-hint
│   ├── handlers_reviews.go     # /api/reviews
│   ├── handlers_admin.go       # /api/admin/* (admin-only)
│   └── handlers_password_reset.go
├── static/
│   ├── index.html              # Single-page app shell
│   ├── css/
│   │   └── main.css            # All styles (dark theme, arena, cards, etc.)
│   └── js/
│       ├── app.js              # App init, auth guard, global state
│       ├── state.js            # Shared mutable state (currentUser, currentQuestion)
│       ├── api.js              # apiFetch() wrapper (auth headers, error handling)
│       ├── auth.js             # Login / register forms
│       ├── tabs.js             # Tab switching (switchTab)
│       ├── editor.js           # CodeMirror init, language switching, autocomplete
│       ├── runner.js           # Run Tests button, renderTestResults()
│       ├── questions.js        # Question list, detail pane, mark complete
│       ├── submissions.js      # Submissions list, rating UI
│       ├── arena.js            # Shared arena view (challenge + tournament)
│       ├── challenges.js       # Challenge tab: list, send/accept/reject, enter arena
│       ├── tournaments.js      # Tournament tab: list, join/leave, enter arena, leaderboard
│       ├── clans.js            # Clan tab: create/join clan, render clan view
│       ├── raids.js            # Raid list and entry
│       ├── raid_arena.js       # Raid arena: question select, editor, live scoreboard
│       ├── social.js           # Follow, feed, search
│       ├── leaderboard.js      # Rankings table
│       ├── hint.js             # Hint toggle, AI hint request
│       ├── admin.js            # Admin panel UI
│       ├── timer.js            # Generic countdown utility
│       ├── contest_timer.js    # ContestTimer (challenge/tournament) + notification poller
│       ├── qtimer.js           # Per-question timer (optional)
│       ├── anticheat.js        # AntiCheat (tab-switch / copy-paste detection)
│       └── utils.js            # escHtml(), showToast(), showConfirmModal(), etc.
├── go.mod
└── render.yaml
```

---

## Getting Started

### Prerequisites

- Go 1.21+
- GCC / CGO (required by `go-sqlite3`)
- Docker (optional, used by the code runner sandbox)

### Local Development

```bash
# Clone the repo
git clone <your-repo-url>
cd hackerthon

# Download dependencies
go mod download

# Run the server (serves static files + API on :8080)
go run ./go

# Open in browser
open http://localhost:8080
```

The database file (`hackerthon.db`) is created automatically on first run with all schema migrations applied.

### First-Time Setup

1. Register an account at `/` — the first registered user is automatically granted admin privileges.
2. Log in and open the **Admin Panel** tab.
3. Create questions, then create challenges/tournaments and assign questions to them.

---

## Configuration

The server reads the following environment variables:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | `./hackerthon.db` | Path to SQLite file |
| `JWT_SECRET` | *(required in prod)* | Secret key for signing JWTs |
| `RUNNER_TIMEOUT` | `10` | Code execution timeout in seconds |

---

## Core Concepts

### Questions

Each question has a title, description, difficulty (`easy`/`medium`/`hard`), category, a static hint + hint URL, and a test suite. Test suites are either:

- **JSON test cases** — an array of `{input, expected_output}` objects; the runner feeds each as stdin and compares stdout
- **Go test file** — a `_test.go` file uploaded and compiled alongside user code (Go submissions only)

Questions can be toggled visible/hidden by admins; hidden questions don't appear in the public list but can still be assigned to contests.

### Code Runner

User code is executed in a sandboxed subprocess (or Docker container if configured). The runner compiles and runs the submitted code against each test case, captures stdout/stderr, compares against expected output, and returns structured pass/fail results. Execution is killed after `RUNNER_TIMEOUT` seconds.

### Challenges (1v1)

1. User A sends a challenge to User B.
2. User B accepts or rejects via the Challenges tab.
3. An admin (or the challenge initiator if allowed) assigns questions and a duration.
4. Both users enter the arena independently; each sees the same questions, their own editor, and their own timer.
5. Submissions are scored; the user who solves the most questions wins.

### Tournaments

1. Admin creates a tournament with a scheduled start time, max participants, and duration.
2. Users join before it starts.
3. At start time, the arena becomes available — participants click **Enter Arena**.
4. Questions are solved for points; a leaderboard shows rankings after the tournament ends.
5. Anti-cheat runs during the session; tab-switching or copy-paste triggers a disqualification warning.

### Clan Raids

1. Users create or join a clan.
2. An admin creates a raid and assigns clans + questions.
3. All clan members enter the raid arena; each member's solves add points to the clan's total.
4. A live scoreboard updates every 5 seconds showing clan rankings.
5. A member who solves all questions early is marked finished and sees a waiting-room view with the live clan scores.

### Anti-Cheat

`anticheat.js` activates `AntiCheat.start(type, id)` when a user enters any arena. It listens for:

- **`visibilitychange`** (tab switch / window minimize)
- **`copy` / `paste`** events

Each violation is logged to the server. Exceeding the threshold triggers a disqualification modal and exits the arena. `AntiCheat.stop()` is called on exit or when the timer expires.

---

## API Reference

All endpoints are prefixed with `/api`. Protected endpoints require a `Authorization: Bearer <jwt>` header.

### Auth

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/register` | Create account `{username, password}` |
| `POST` | `/api/login` | Get JWT `{username, password}` |
| `GET` | `/api/me` | Current user info |

### Questions

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/questions` | List visible questions |
| `POST` | `/api/questions` | Create question *(admin)* |
| `PUT` | `/api/questions/:id` | Update question *(admin)* |
| `DELETE` | `/api/questions/:id` | Delete question *(admin)* |
| `POST` | `/api/questions/:id/toggle-visibility` | Show/hide *(admin)* |

### Code Execution

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/questions/:id/run` | Run code against test cases |
| `POST` | `/api/questions/:id/submit` | Submit solution |

### Hints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/hints/:questionId/status` | AI hint usage status for user |
| `POST` | `/api/hints/:questionId/ai` | Request AI hint (rate-limited) |

### Submissions & Reviews

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/questions/:id/submissions` | List submissions for a question |
| `POST` | `/api/submissions/:id/review` | Rate a submission |

### Challenges

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/challenges` | List challenges involving current user |
| `POST` | `/api/challenges` | Send a challenge `{opponent_id}` |
| `POST` | `/api/challenges/:id/accept` | Accept a challenge |
| `DELETE` | `/api/challenges/:id/reject` | Reject a challenge |
| `POST` | `/api/challenges/:id/enter` | Enter challenge arena |
| `POST` | `/api/challenges/:id/arena-submit` | Submit solution inside arena |

### Tournaments

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/tournaments` | List all tournaments |
| `POST` | `/api/tournaments` | Create tournament *(admin)* |
| `POST` | `/api/tournaments/:id/join` | Join upcoming tournament |
| `DELETE` | `/api/tournaments/:id/leave` | Leave tournament |
| `POST` | `/api/tournaments/:id/enter` | Enter active arena |
| `POST` | `/api/tournaments/:id/arena-submit` | Submit inside arena |
| `GET` | `/api/tournaments/:id/leaderboard` | Results |

### Clans & Raids

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/clans` | List clans |
| `POST` | `/api/clans` | Create clan |
| `POST` | `/api/clans/:id/join` | Join a clan |
| `DELETE` | `/api/clans/:id/leave` | Leave a clan |
| `GET` | `/api/raids` | List raids |
| `POST` | `/api/raids/:id/enter` | Enter raid arena |
| `POST` | `/api/raids/:id/arena-submit` | Submit inside raid |
| `GET` | `/api/raids/:id/leaderboard` | Live clan scores |
| `POST` | `/api/raids/:id/finish` | Mark self as finished |

### Social

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/follow/:userId` | Follow a user |
| `DELETE` | `/api/follow/:userId` | Unfollow |
| `GET` | `/api/feed` | Activity feed from followed users |
| `GET` | `/api/search?q=` | Search users by username |
| `GET` | `/api/leaderboard` | Global rankings |

### Admin

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/admin/users` | List all users |
| `POST` | `/api/admin/users/:id/promote` | Grant admin |
| `POST` | `/api/admin/challenges/:id/questions` | Assign questions to challenge |
| `POST` | `/api/admin/tournaments/:id/questions` | Assign questions to tournament |

---

## Frontend Architecture

The app is a single HTML page (`index.html`) with a tab-based navigation. Each tab is a hidden `div` that becomes visible on click via `switchTab()` in `tabs.js`.

**Global state** lives in `state.js`:
- `currentUser` — logged-in user object (null if not authed)
- `currentQuestion` — currently viewed question (kept in sync across arena modes so `hint.js` works correctly)

**Tab initialisation** — `switchTab(name)` calls the appropriate `init*Tab()` function when a tab is activated. Arena modes (challenge, tournament, raid) replace the entire tab content with their own HTML when entered, then restore the list view on exit.

**Editor lifecycle** — `initCodeEditor()` in `editor.js` destroys any existing CodeMirror instance, clears the container, then mounts a fresh instance inside `requestAnimationFrame` + `setTimeout(50)` to ensure the browser has fully painted the new DOM before CodeMirror measures the container. This is critical because CodeMirror reads container dimensions at mount time; mounting in a zero-height or zero-width container produces a broken layout.

**Test runner flow**:
1. User clicks **Run Tests** → `runTests(qid)` in `runner.js`
2. `apiFetch` POSTs code + language to `/api/questions/:id/run`
3. Response is passed to `renderTestResults()` which renders per-case pass/fail cards
4. If all cases pass, the **Submit Solution** button is enabled

---

## Admin Panel

Accessible via the **Admin Panel** tab (visible only to admin users).

- **Users** — view all registered users; promote any user to admin
- **Questions** — full list with visibility toggles and delete; add new questions with the form at the bottom (supports JSON test cases or a Go test file path)
- **Challenges** — view pending challenges; assign question sets
- **Tournaments** — create tournaments, assign questions + duration
- **Raids** — create raids, assign clans and questions

---

## Deployment

The project includes a `render.yaml` for one-click deployment on [Render](https://render.com).

```yaml
# render.yaml (summary)
services:
  - type: web
    name: hackerthon
    env: go
    buildCommand: go build -o server ./go
    startCommand: ./server
```

Set the following environment variables in your Render dashboard:

- `JWT_SECRET` — a long random string (e.g. `openssl rand -hex 32`)
- `DATABASE_URL` — leave as default to use the local SQLite file, or point to a persistent disk mount

For persistent storage on Render, attach a **Disk** to the service and set `DATABASE_URL` to a path on that disk (e.g. `/data/hackerthon.db`), otherwise the database resets on every deploy.
