package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// ── MODELS ────────────────────────────────────────────────
type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type Question struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Difficulty  string `json:"difficulty"`
	Category    string `json:"category"`
	HintURL     string `json:"hint_url"`
	HintText    string `json:"hint_text"`
	Visible     bool   `json:"visible"`
	TestCases   string `json:"test_cases"` // JSON array of {input, expected} — used by Python/JS/bash
	TestFile    string `json:"test_file"`  // Full _test.go content — used by Go questions
}

type Submission struct {
	ID          int       `json:"id"`
	QuestionID  int       `json:"question_id"`
	UserID      int       `json:"user_id"`
	AuthorName  string    `json:"author_name"`
	Code        string    `json:"code"`
	Language    string    `json:"language"`
	Notes       string    `json:"notes"`
	AvgRating   float64   `json:"avg_rating"`
	ReviewCount int       `json:"review_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type Review struct {
	ID           int       `json:"id"`
	SubmissionID int       `json:"submission_id"`
	UserID       int       `json:"user_id"`
	ReviewerName string    `json:"reviewer_name"`
	Rating       int       `json:"rating"`
	Comment      string    `json:"comment"`
	CreatedAt    time.Time `json:"created_at"`
}

type Contributor struct {
	UserID          int     `json:"user_id"`
	AuthorName      string  `json:"author_name"`
	SubmissionCount int     `json:"submission_count"`
	AvgRating       float64 `json:"avg_rating"`
	TotalReviews    int     `json:"total_reviews"`
}

type TestCase struct {
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

type TestResult struct {
	Index    int    `json:"index"`
	Passed   bool   `json:"passed"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Got      string `json:"got"`
	Error    string `json:"error,omitempty"`
}

type RunResult struct {
	Passed    int          `json:"passed"`
	Total     int          `json:"total"`
	AllPassed bool         `json:"all_passed"`
	Results   []TestResult `json:"results"`
}

var db *sql.DB

// ── DB SETUP ──────────────────────────────────────────────
func initDB() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL environment variable not set")
	}
	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("opening db: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("pinging db: %w", err)
	}
	return createTables()
}

func createTables() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            SERIAL PRIMARY KEY,
			name          TEXT NOT NULL,
			email         TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_admin      BOOLEAN NOT NULL DEFAULT FALSE,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS sessions (
			id         SERIAL PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token      TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS follows (
			follower_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			followee_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (follower_id, followee_id)
		);

		CREATE TABLE IF NOT EXISTS questions (
			id          SERIAL PRIMARY KEY,
			title       TEXT NOT NULL,
			description TEXT NOT NULL,
			difficulty  TEXT NOT NULL DEFAULT 'medium',
			category    TEXT NOT NULL DEFAULT 'General',
			hint_url    TEXT NOT NULL DEFAULT 'https://pkg.go.dev',
			hint_text   TEXT NOT NULL DEFAULT 'Go documentation',
			visible     BOOLEAN NOT NULL DEFAULT FALSE
		);

		CREATE TABLE IF NOT EXISTS submissions (
			id          SERIAL PRIMARY KEY,
			question_id INTEGER NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
			user_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
			author_name TEXT NOT NULL,
			code        TEXT NOT NULL,
			language    TEXT NOT NULL DEFAULT 'go',
			notes       TEXT NOT NULL DEFAULT '',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS reviews (
			id            SERIAL PRIMARY KEY,
			submission_id INTEGER NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
			user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL,
			reviewer_name TEXT NOT NULL,
			rating        INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
			comment       TEXT NOT NULL DEFAULT '',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}

	// Migrations for existing DBs
	db.Exec(`ALTER TABLE questions ADD COLUMN IF NOT EXISTS visible BOOLEAN NOT NULL DEFAULT FALSE`)
	db.Exec(`ALTER TABLE submissions ADD COLUMN IF NOT EXISTS user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`)
	db.Exec(`ALTER TABLE reviews ADD COLUMN IF NOT EXISTS user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`)
	db.Exec(`ALTER TABLE questions ADD COLUMN IF NOT EXISTS test_cases TEXT NOT NULL DEFAULT '[]'`)
	// New: test_file stores full _test.go content for Go questions
	db.Exec(`ALTER TABLE questions ADD COLUMN IF NOT EXISTS test_file TEXT NOT NULL DEFAULT ''`)

	if err := seedAdminUser(); err != nil {
		log.Printf("admin seed: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM questions`).Scan(&count)
	if count == 0 {
		return seedQuestions()
	}
	return nil
}

func seedAdminUser() error {
	var exists bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, "victorakor04@gmail.com").Scan(&exists)
	if exists {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("VicTorAKor3"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO users (name, email, password_hash, is_admin) VALUES ($1,$2,$3,$4)`,
		"Victor", "victorakor04@gmail.com", string(hash), true)
	if err != nil {
		return fmt.Errorf("seeding admin: %w", err)
	}
	log.Println("✓ Admin user seeded")
	return nil
}

func seedQuestions() error {
	seeds := []Question{
		{Title: "Reading a File with os.ReadFile", Description: "Write a Go program that accepts a filename as a command-line argument (os.Args), reads the entire content of the file, and prints it to stdout. Handle the case where no argument is provided or the file doesn't exist with a meaningful error message.", Difficulty: "easy", Category: "File I/O", HintURL: "https://pkg.go.dev/os#ReadFile", HintText: "os.ReadFile docs"},
		{Title: "Writing Output to a File", Description: "Extend your file reader: now accept TWO command-line arguments (input file, output file). Read the input file, convert all text to uppercase using strings.ToUpper, and write the result to the output file using os.WriteFile. Make sure to use the correct file permission flags.", Difficulty: "easy", Category: "File I/O", HintURL: "https://pkg.go.dev/os#WriteFile", HintText: "os.WriteFile docs"},
		{Title: "Hex to Decimal Conversion", Description: "Write a function `hexToDec(s string) (int64, error)` that converts a hexadecimal string to its decimal integer equivalent. Test it with inputs like '1E' (should return 30), 'FF' (should return 255), and 'BADF00D'. Use only the standard library.", Difficulty: "easy", Category: "Strconv & Numbers", HintURL: "https://pkg.go.dev/strconv#ParseInt", HintText: "strconv.ParseInt with base 16"},
		{Title: "Binary to Decimal Conversion", Description: "Write a function `binToDec(s string) (int64, error)` that converts a binary string to its decimal equivalent. Test it with '10' (should return 2), '1010' (should return 10), '11111111' (should return 255). Think about what base to pass to strconv.ParseInt.", Difficulty: "easy", Category: "Strconv & Numbers", HintURL: "https://pkg.go.dev/strconv#ParseInt", HintText: "strconv.ParseInt with base 2"},
		{Title: "Splitting Text Into Tokens", Description: "Write a function that takes a string and returns a slice of all words/tokens split by whitespace. Then write a second version that preserves punctuation as separate tokens. Compare the outputs for: 'Hello, world! How are you?' Hint: look at strings.Fields vs strings.Split.", Difficulty: "easy", Category: "String Manipulation", HintURL: "https://pkg.go.dev/strings#Fields", HintText: "strings.Fields and strings.Split docs"},
		{Title: "Capitalizing Words", Description: "Write a function `capitalize(s string) string` that capitalizes only the first letter of a word (making the rest lowercase). Then write `capitalizeN(words []string, n int) []string` that capitalizes the last N words in the slice. This mimics the (cap, N) modifier in go-reloaded.", Difficulty: "medium", Category: "String Manipulation", HintURL: "https://pkg.go.dev/strings#Title", HintText: "strings.ToUpper on runes + unicode.ToLower"},
		{Title: "Detecting Vowel-Starting Words (a vs an)", Description: "Write a function `fixArticles(words []string) []string` that scans a word slice and replaces 'a' with 'an' whenever the next word starts with a vowel (a, e, i, o, u) or the letter 'h'. Handle both uppercase and lowercase 'A'/'a'.", Difficulty: "medium", Category: "String Manipulation", HintURL: "https://pkg.go.dev/strings#ContainsRune", HintText: "strings.ContainsRune or strings.IndexRune"},
		{Title: "Punctuation Spacing Fixer", Description: "Write a function that takes a string and ensures punctuation marks (. , ! ? : ;) are directly attached to the preceding word with exactly one space after them. Example: 'Hello , world !' becomes 'Hello, world!'. Also handle groups like '...' and '!?' that should stay together.", Difficulty: "medium", Category: "String Manipulation", HintURL: "https://pkg.go.dev/strings#TrimSpace", HintText: "strings.Builder for efficient string construction"},
		{Title: "Single Quote Formatter", Description: "Write a function that finds pairs of single quotes in a string and removes spaces between the quotes and the enclosed text. Example: `' awesome '` becomes `'awesome'` and `' hello world '` becomes `'hello world'`. Think about how to track odd/even quote occurrences.", Difficulty: "medium", Category: "String Manipulation", HintURL: "https://pkg.go.dev/strings#Index", HintText: "strings.Index + manual index tracking"},
		{Title: "Loading ASCII Banner from File", Description: "Write a function that reads one of the ascii-art banner files (standard, shadow, thinkertoy) and parses it into a map[rune][8]string — mapping each ASCII character to its 8 lines of art. Remember: each character is 8 lines tall, separated by a newline.", Difficulty: "medium", Category: "File I/O", HintURL: "https://pkg.go.dev/bufio#Scanner", HintText: "bufio.Scanner with ScanLines to read line by line"},
		{Title: "Rendering ASCII Art for a String", Description: "Using the map you built in the previous question, write `renderASCII(input string, banner map[rune][8]string) string` that takes a string and returns the multi-line ASCII art representation. Each output line should be built by concatenating the corresponding art line for each character.", Difficulty: "medium", Category: "Data Manipulation", HintURL: "https://pkg.go.dev/strings#Builder", HintText: "strings.Builder for efficient multi-line construction"},
		{Title: "Handling \\n in ASCII Art Input", Description: "Modify your ASCII renderer to handle literal '\\n' in the input string (e.g. 'Hello\\nWorld'). When you encounter '\\n', finish the current line group and start a new one. An empty string between two '\\n\\n' should produce a blank 8-line block.", Difficulty: "medium", Category: "Data Manipulation", HintURL: "https://pkg.go.dev/strings#Split", HintText: "strings.Split on \\n then process each segment"},
		{Title: "Regex Pattern Matching for Modifiers", Description: "Write a function using regexp to find all occurrences of modifier patterns like (hex), (bin), (up), (low), (cap), (up, 2), (low, 3), (cap, 5) in a string. Return each match along with its position and the optional number argument if present.", Difficulty: "hard", Category: "Regex & Parsing", HintURL: "https://pkg.go.dev/regexp", HintText: "regexp.MustCompile with named capture groups"},
		{Title: "Go Error Handling Pattern", Description: "Refactor a function that does: read file → parse content → write output into idiomatic Go style. Each step should return an error. The caller should use if err != nil checks. Wrap errors with fmt.Errorf and %w for context. Demonstrate errors.Is() usage.", Difficulty: "medium", Category: "Error Handling", HintURL: "https://go.dev/blog/error-handling-and-go", HintText: "Go error handling blog post"},
		{Title: "Writing Unit Tests in Go", Description: "Write a _test.go file that tests your hexToDec and binToDec functions using table-driven tests. Create a struct with fields: input string, expected int64, wantErr bool. Use t.Run() for subtests and t.Errorf() to report failures. Run with go test ./...", Difficulty: "medium", Category: "Testing", HintURL: "https://pkg.go.dev/testing", HintText: "Go testing package + table-driven test pattern"},
		{Title: "HTTP Server with net/http", Description: "Write a Go HTTP server that listens on port 8080. Register two routes: GET / serves an HTML page, and POST /convert accepts a JSON body {\"text\": \"...\", \"type\": \"hex|bin\"} and returns the converted result as JSON. Use only the standard library.", Difficulty: "hard", Category: "HTTP & Networking", HintURL: "https://pkg.go.dev/net/http", HintText: "net/http.HandleFunc + json.NewDecoder"},
		{Title: "Structs, Methods and Interfaces", Description: "Design a TextProcessor struct that holds a slice of Transformer interfaces. Each Transformer has a single method: Transform(words []string) []string. Implement at least two concrete transformers: UpperCaseTransformer and HexTransformer. Chain them in the processor's Run() method.", Difficulty: "hard", Category: "Structs & Interfaces", HintURL: "https://go.dev/tour/methods/9", HintText: "Go interfaces tour"},
		{Title: "Runes vs Bytes — Unicode Safety", Description: "Write two versions of a 'capitalize first letter' function: one using byte indexing (s[0]) and one using []rune(s). Test both with ASCII strings AND a string starting with a multi-byte unicode character like 'ñoño'. Explain why byte indexing can corrupt unicode strings.", Difficulty: "hard", Category: "String Manipulation", HintURL: "https://go.dev/blog/strings", HintText: "Go strings, bytes, runes blog"},
		{Title: "Goroutines: Parallel File Processing", Description: "Given a slice of 5 filenames, process each file concurrently using goroutines. Each goroutine should read the file and send its word count to a channel. The main goroutine collects all results and prints a summary. Use sync.WaitGroup OR a done channel to know when all are finished.", Difficulty: "hard", Category: "Concurrency", HintURL: "https://go.dev/tour/concurrency/1", HintText: "Go goroutines and channels tour"},
		{Title: "Full Pipeline: go-reloaded Mini", Description: "FINAL BOSS: Build a mini version of go-reloaded that handles ONLY these three transformations: (hex), (bin), and (up). Accept input/output filenames from os.Args. Read the input file, apply all three transformations in a single pass over the token slice, write the result. All edge cases handled.", Difficulty: "hard", Category: "Full Project", HintURL: "https://pkg.go.dev/strings", HintText: "Combine os, strings, strconv packages"},
	}
	for _, q := range seeds {
		_, err := db.Exec(`INSERT INTO questions (title,description,difficulty,category,hint_url,hint_text,visible,test_cases,test_file) VALUES ($1,$2,$3,$4,$5,$6,FALSE,'[]','')`,
			q.Title, q.Description, q.Difficulty, q.Category, q.HintURL, q.HintText)
		if err != nil {
			return fmt.Errorf("seeding %q: %w", q.Title, err)
		}
	}
	log.Printf("✓ Seeded %d questions", len(seeds))
	return nil
}

// ── AUTH HELPERS ──────────────────────────────────────────
func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getUserFromToken(r *http.Request) (*User, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, fmt.Errorf("no token")
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	var u User
	err := db.QueryRow(`
		SELECT u.id, u.name, u.email, u.is_admin
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token=$1 AND s.expires_at > NOW()`, token).
		Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	return &u, nil
}

func requireAuth(w http.ResponseWriter, r *http.Request) *User {
	u, err := getUserFromToken(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return nil
	}
	return u
}

func requireAdmin(w http.ResponseWriter, r *http.Request) *User {
	u := requireAuth(w, r)
	if u == nil {
		return nil
	}
	if !u.IsAdmin {
		http.Error(w, "forbidden", 403)
		return nil
	}
	return u
}

// ── AUTH HANDLERS ─────────────────────────────────────────
func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if body.Name == "" || body.Email == "" || body.Password == "" {
		http.Error(w, "name, email and password required", 400)
		return
	}
	if len(body.Password) < 6 {
		http.Error(w, "password must be at least 6 characters", 400)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "server error", 500)
		return
	}
	var u User
	err = db.QueryRow(`INSERT INTO users (name,email,password_hash) VALUES ($1,$2,$3) RETURNING id,name,email,is_admin,created_at`,
		body.Name, body.Email, string(hash)).
		Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			http.Error(w, "email already registered", 409)
			return
		}
		http.Error(w, "server error", 500)
		return
	}
	token := generateToken()
	db.Exec(`INSERT INTO sessions (user_id,token,expires_at) VALUES ($1,$2,$3)`,
		u.ID, token, time.Now().Add(30*24*time.Hour))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{"user": u, "token": token})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	var u User
	var hash string
	err := db.QueryRow(`SELECT id,name,email,password_hash,is_admin,created_at FROM users WHERE email=$1`, body.Email).
		Scan(&u.ID, &u.Name, &u.Email, &hash, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		http.Error(w, "invalid email or password", 401)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		http.Error(w, "invalid email or password", 401)
		return
	}
	token := generateToken()
	db.Exec(`INSERT INTO sessions (user_id,token,expires_at) VALUES ($1,$2,$3)`,
		u.ID, token, time.Now().Add(30*24*time.Hour))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"user": u, "token": token})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		db.Exec(`DELETE FROM sessions WHERE token=$1`, token)
	}
	w.WriteHeader(204)
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := requireAuth(w, r)
	if u == nil {
		return
	}
	var answered int
	db.QueryRow(`SELECT COUNT(DISTINCT question_id) FROM submissions WHERE user_id=$1`, u.ID).Scan(&answered)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user":     u,
		"answered": answered,
	})
}

// ── ADMIN HANDLERS ────────────────────────────────────────
func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if requireAdmin(w, r) == nil {
			return
		}
		rows, err := db.Query(`SELECT id,name,email,is_admin,created_at FROM users ORDER BY created_at`)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var users []User
		for rows.Next() {
			var u User
			rows.Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin, &u.CreatedAt)
			users = append(users, u)
		}
		if users == nil {
			users = []User{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)

	case http.MethodDelete:
		admin := requireAdmin(w, r)
		if admin == nil {
			return
		}
		var body struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if body.ID == admin.ID {
			http.Error(w, "cannot delete yourself", 400)
			return
		}
		res, err := db.Exec(`DELETE FROM users WHERE id=$1`, body.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			http.Error(w, "not found", 404)
			return
		}
		w.WriteHeader(204)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func handleAdminPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if requireAdmin(w, r) == nil {
		return
	}
	var body struct {
		ID      int  `json:"id"`
		IsAdmin bool `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	_, err := db.Exec(`UPDATE users SET is_admin=$1 WHERE id=$2`, body.IsAdmin, body.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

// ── QUESTIONS ─────────────────────────────────────────────
func handleQuestions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		u, _ := getUserFromToken(r)
		var rows *sql.Rows
		var err error
		if u != nil && u.IsAdmin {
			rows, err = db.Query(`SELECT id,title,description,difficulty,category,hint_url,hint_text,visible,COALESCE(test_cases,'[]'),COALESCE(test_file,'') FROM questions ORDER BY id`)
		} else {
			rows, err = db.Query(`SELECT id,title,description,difficulty,category,hint_url,hint_text,visible,COALESCE(test_cases,'[]'),COALESCE(test_file,'') FROM questions WHERE visible=TRUE ORDER BY id`)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var qs []Question
		for rows.Next() {
			var q Question
			rows.Scan(&q.ID, &q.Title, &q.Description, &q.Difficulty, &q.Category, &q.HintURL, &q.HintText, &q.Visible, &q.TestCases, &q.TestFile)
			qs = append(qs, q)
		}
		if qs == nil {
			qs = []Question{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qs)

	case http.MethodPost:
		if requireAdmin(w, r) == nil {
			return
		}
		var q Question
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if q.Title == "" || q.Description == "" {
			http.Error(w, "title and description required", 400)
			return
		}
		if q.Difficulty == "" {
			q.Difficulty = "medium"
		}
		if q.Category == "" {
			q.Category = "General"
		}
		if q.HintURL == "" {
			q.HintURL = "https://pkg.go.dev"
		}
		if q.HintText == "" {
			q.HintText = "Go documentation"
		}
		if q.TestCases == "" {
			q.TestCases = "[]"
		}
		// Validate test_cases is valid JSON when provided
		var tc []TestCase
		if err := json.Unmarshal([]byte(q.TestCases), &tc); err != nil {
			http.Error(w, "test_cases must be a valid JSON array", 400)
			return
		}
		// test_file is stored as-is (plain Go test source); no validation needed here
		err := db.QueryRow(
			`INSERT INTO questions (title,description,difficulty,category,hint_url,hint_text,visible,test_cases,test_file)
			 VALUES ($1,$2,$3,$4,$5,$6,FALSE,$7,$8) RETURNING id`,
			q.Title, q.Description, q.Difficulty, q.Category, q.HintURL, q.HintText, q.TestCases, q.TestFile,
		).Scan(&q.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(q)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func handleDeleteQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", 405)
		return
	}
	if requireAdmin(w, r) == nil {
		return
	}
	var p struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	res, err := db.Exec(`DELETE FROM questions WHERE id=$1`, p.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Error(w, "not found", 404)
		return
	}
	w.WriteHeader(204)
}

func handleQuestionVisibility(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", 405)
		return
	}
	if requireAdmin(w, r) == nil {
		return
	}
	var body struct {
		ID      int  `json:"id"`
		Visible bool `json:"visible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	_, err := db.Exec(`UPDATE questions SET visible=$1 WHERE id=$2`, body.Visible, body.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

// ── SUBMISSIONS ───────────────────────────────────────────
func handleSubmissions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		qid := r.URL.Query().Get("question_id")
		var rows *sql.Rows
		var err error
		if qid != "" {
			rows, err = db.Query(`
				SELECT s.id, s.question_id, COALESCE(s.user_id,0), s.author_name, s.code, s.language, s.notes, s.created_at,
				       COALESCE(AVG(rv.rating),0) as avg_rating,
				       COUNT(rv.id) as review_count
				FROM submissions s
				LEFT JOIN reviews rv ON rv.submission_id = s.id
				WHERE s.question_id = $1
				GROUP BY s.id
				ORDER BY avg_rating DESC, s.created_at ASC`, qid)
		} else {
			rows, err = db.Query(`
				SELECT s.id, s.question_id, COALESCE(s.user_id,0), s.author_name, s.code, s.language, s.notes, s.created_at,
				       COALESCE(AVG(rv.rating),0) as avg_rating,
				       COUNT(rv.id) as review_count
				FROM submissions s
				LEFT JOIN reviews rv ON rv.submission_id = s.id
				GROUP BY s.id
				ORDER BY avg_rating DESC, s.created_at ASC`)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var subs []Submission
		for rows.Next() {
			var s Submission
			rows.Scan(&s.ID, &s.QuestionID, &s.UserID, &s.AuthorName, &s.Code, &s.Language, &s.Notes, &s.CreatedAt, &s.AvgRating, &s.ReviewCount)
			subs = append(subs, s)
		}
		if subs == nil {
			subs = []Submission{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(subs)

	case http.MethodPost:
		u := requireAuth(w, r)
		if u == nil {
			return
		}
		var s Submission
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if s.QuestionID == 0 || s.Code == "" {
			http.Error(w, "question_id and code required", 400)
			return
		}
		s.AuthorName = u.Name
		s.UserID = u.ID
		if s.Language == "" {
			s.Language = "go"
		}
		err := db.QueryRow(`INSERT INTO submissions (question_id,user_id,author_name,code,language,notes) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id,created_at`,
			s.QuestionID, u.ID, u.Name, s.Code, s.Language, s.Notes).Scan(&s.ID, &s.CreatedAt)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(s)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func handleDeleteSubmission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := requireAuth(w, r)
	if u == nil {
		return
	}
	var p struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	var ownerID int
	db.QueryRow(`SELECT COALESCE(user_id,0) FROM submissions WHERE id=$1`, p.ID).Scan(&ownerID)
	if !u.IsAdmin && ownerID != u.ID {
		http.Error(w, "forbidden", 403)
		return
	}
	res, err := db.Exec(`DELETE FROM submissions WHERE id=$1`, p.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Error(w, "not found", 404)
		return
	}
	w.WriteHeader(204)
}

// ── REVIEWS ───────────────────────────────────────────────
func handleReviews(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sid := r.URL.Query().Get("submission_id")
		if sid == "" {
			http.Error(w, "submission_id required", 400)
			return
		}
		rows, err := db.Query(`SELECT id,submission_id,COALESCE(user_id,0),reviewer_name,rating,comment,created_at FROM reviews WHERE submission_id=$1 ORDER BY created_at DESC`, sid)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var reviews []Review
		for rows.Next() {
			var rv Review
			rows.Scan(&rv.ID, &rv.SubmissionID, &rv.UserID, &rv.ReviewerName, &rv.Rating, &rv.Comment, &rv.CreatedAt)
			reviews = append(reviews, rv)
		}
		if reviews == nil {
			reviews = []Review{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reviews)

	case http.MethodPost:
		u := requireAuth(w, r)
		if u == nil {
			return
		}
		var rv Review
		if err := json.NewDecoder(r.Body).Decode(&rv); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if rv.SubmissionID == 0 || rv.Rating < 1 || rv.Rating > 5 {
			http.Error(w, "submission_id and rating(1-5) required", 400)
			return
		}
		rv.ReviewerName = u.Name
		rv.UserID = u.ID
		err := db.QueryRow(`INSERT INTO reviews (submission_id,user_id,reviewer_name,rating,comment) VALUES ($1,$2,$3,$4,$5) RETURNING id,created_at`,
			rv.SubmissionID, u.ID, u.Name, rv.Rating, rv.Comment).Scan(&rv.ID, &rv.CreatedAt)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(rv)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// ── LEADERBOARD ───────────────────────────────────────────
func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	rows, err := db.Query(`
		SELECT COALESCE(s.user_id,0), s.author_name,
		       COUNT(DISTINCT s.id) as submission_count,
		       COALESCE(AVG(rv.rating),0) as avg_rating,
		       COUNT(rv.id) as total_reviews
		FROM submissions s
		LEFT JOIN reviews rv ON rv.submission_id = s.id
		GROUP BY s.user_id, s.author_name
		ORDER BY avg_rating DESC, submission_count DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var board []Contributor
	for rows.Next() {
		var c Contributor
		rows.Scan(&c.UserID, &c.AuthorName, &c.SubmissionCount, &c.AvgRating, &c.TotalReviews)
		board = append(board, c)
	}
	if board == nil {
		board = []Contributor{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(board)
}

// ── SEARCH ────────────────────────────────────────────────
func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if requireAuth(w, r) == nil {
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name required", 400)
		return
	}

	type UserProfile struct {
		User        User         `json:"user"`
		Submissions []Submission `json:"submissions"`
	}

	rows, err := db.Query(`SELECT id,name,email,is_admin,created_at FROM users WHERE LOWER(name) LIKE LOWER($1) ORDER BY name`, "%"+name+"%")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var results []UserProfile = []UserProfile{}

	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin, &u.CreatedAt); err != nil {
			continue
		}

		var subs []Submission
		func() {
			subRows, err := db.Query(`
				SELECT s.id, s.question_id, COALESCE(s.user_id,0), s.author_name, s.code, s.language, s.notes, s.created_at,
				       COALESCE(AVG(rv.rating),0), COUNT(rv.id)
				FROM submissions s
				LEFT JOIN reviews rv ON rv.submission_id = s.id
				WHERE s.user_id = $1
				GROUP BY s.id ORDER BY s.created_at DESC`, u.ID)
			if err != nil {
				return
			}
			defer subRows.Close()
			for subRows.Next() {
				var s Submission
				subRows.Scan(&s.ID, &s.QuestionID, &s.UserID, &s.AuthorName, &s.Code, &s.Language, &s.Notes, &s.CreatedAt, &s.AvgRating, &s.ReviewCount)
				subs = append(subs, s)
			}
		}()

		if subs == nil {
			subs = []Submission{}
		}
		results = append(results, UserProfile{User: u, Submissions: subs})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ── FOLLOWS ───────────────────────────────────────────────
func handleFollows(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		u := requireAuth(w, r)
		if u == nil {
			return
		}
		var body struct {
			FolloweeID int `json:"followee_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if body.FolloweeID == u.ID {
			http.Error(w, "cannot follow yourself", 400)
			return
		}
		db.Exec(`INSERT INTO follows (follower_id,followee_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, u.ID, body.FolloweeID)
		w.WriteHeader(204)

	case http.MethodDelete:
		u := requireAuth(w, r)
		if u == nil {
			return
		}
		var body struct {
			FolloweeID int `json:"followee_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		db.Exec(`DELETE FROM follows WHERE follower_id=$1 AND followee_id=$2`, u.ID, body.FolloweeID)
		w.WriteHeader(204)

	case http.MethodGet:
		u := requireAuth(w, r)
		if u == nil {
			return
		}
		rows, err := db.Query(`
			SELECT u.id FROM follows f JOIN users u ON u.id = f.followee_id
			WHERE f.follower_id = $1`, u.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			rows.Scan(&id)
			ids = append(ids, id)
		}
		if ids == nil {
			ids = []int{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ids)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// ── NOTIFICATIONS ─────────────────────────────────────────
func handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := requireAuth(w, r)
	if u == nil {
		return
	}
	type Notification struct {
		Submission Submission `json:"submission"`
		Question   Question   `json:"question"`
	}
	rows, err := db.Query(`
		SELECT s.id, s.question_id, COALESCE(s.user_id,0), s.author_name, s.code, s.language, s.notes, s.created_at,
		       COALESCE(AVG(rv.rating),0), COUNT(rv.id),
		       q.id, q.title, q.description, q.difficulty, q.category, q.hint_url, q.hint_text, q.visible
		FROM submissions s
		JOIN follows f ON f.followee_id = s.user_id
		JOIN questions q ON q.id = s.question_id
		LEFT JOIN reviews rv ON rv.submission_id = s.id
		WHERE f.follower_id = $1
		GROUP BY s.id, q.id
		ORDER BY s.created_at DESC
		LIMIT 50`, u.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var notifs []Notification
	for rows.Next() {
		var n Notification
		rows.Scan(
			&n.Submission.ID, &n.Submission.QuestionID, &n.Submission.UserID,
			&n.Submission.AuthorName, &n.Submission.Code, &n.Submission.Language,
			&n.Submission.Notes, &n.Submission.CreatedAt, &n.Submission.AvgRating, &n.Submission.ReviewCount,
			&n.Question.ID, &n.Question.Title, &n.Question.Description,
			&n.Question.Difficulty, &n.Question.Category, &n.Question.HintURL,
			&n.Question.HintText, &n.Question.Visible,
		)
		notifs = append(notifs, n)
	}
	if notifs == nil {
		notifs = []Notification{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notifs)
}

// ── SANDBOX CODE RUNNER ───────────────────────────────────

// execSemaphore limits concurrent code executions to protect server resources.
var execSemaphore = make(chan struct{}, 10)

// acquireSemaphore tries to acquire a slot within 5 seconds, returns false if busy.
func acquireSemaphore() bool {
	select {
	case execSemaphore <- struct{}{}:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

func runCode(language, code, input string) (stdout string, runErr string) {
	if !acquireSemaphore() {
		return "", "server busy — too many concurrent executions, try again in a moment"
	}
	defer func() { <-execSemaphore }()

	dir, err := os.MkdirTemp("", "run-*")
	if err != nil {
		return "", "failed to create temp dir"
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	switch language {
	case "go":
		f := filepath.Join(dir, "main.go")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			return "", "failed to write file"
		}
		cmd = exec.CommandContext(ctx, "go", "run", f)

	case "python":
		f := filepath.Join(dir, "main.py")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			return "", "failed to write file"
		}
		cmd = exec.CommandContext(ctx, "python3", f)

	case "javascript":
		f := filepath.Join(dir, "main.js")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			return "", "failed to write file"
		}
		cmd = exec.CommandContext(ctx, "node", "--max-old-space-size=64", f)

	case "bash":
		f := filepath.Join(dir, "main.sh")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			return "", "failed to write file"
		}
		cmd = exec.CommandContext(ctx, "bash", f)

	default:
		return "", fmt.Sprintf("language '%s' is not supported for test execution — submit without running to save anyway", language)
	}

	cmd.Stdin = bytes.NewBufferString(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	execErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return "", "time limit exceeded (10s)"
	}
	if execErr != nil {
		errMsg := strings.TrimSpace(errBuf.String())
		if errMsg == "" {
			errMsg = execErr.Error()
		}
		return "", errMsg
	}

	return strings.TrimSpace(outBuf.String()), ""
}

// runGoTest compiles and runs a Go test file against the user's submitted code.
// It writes main.go (user code) + main_test.go (question's test file) into a
// temp module, runs `go test -v ./...`, and parses the -v output into TestResults.
func runGoTest(userCode, testFile string) RunResult {
	if !acquireSemaphore() {
		return RunResult{
			Results: []TestResult{{Index: 1, Error: "server busy — too many concurrent executions, try again in a moment"}},
		}
	}
	defer func() { <-execSemaphore }()

	dir, err := os.MkdirTemp("", "gotest-*")
	if err != nil {
		return RunResult{Results: []TestResult{{Index: 1, Error: "failed to create temp dir"}}}
	}
	defer os.RemoveAll(dir)

	// Write go.mod so `go test` works without a GOPATH module
	goMod := "module submission\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		return RunResult{Results: []TestResult{{Index: 1, Error: "failed to write go.mod"}}}
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(userCode), 0644); err != nil {
		return RunResult{Results: []TestResult{{Index: 1, Error: "failed to write main.go"}}}
	}
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(testFile), 0644); err != nil {
		return RunResult{Results: []TestResult{{Index: 1, Error: "failed to write main_test.go"}}}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-v", "./...")
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	cmd.Run() // We intentionally ignore the top-level error; we parse output instead.

	if ctx.Err() == context.DeadlineExceeded {
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return RunResult{Results: []TestResult{{Index: 1, Error: "time limit exceeded (30s)"}}}
	}

	// Combine stdout + stderr for parsing (go test -v writes test lines to stdout,
	// but compilation errors go to stderr).
	combined := outBuf.String()
	if strings.TrimSpace(combined) == "" {
		// Compilation failure — stderr has the details
		errMsg := strings.TrimSpace(errBuf.String())
		if errMsg == "" {
			errMsg = "compilation failed (no output)"
		}
		return RunResult{Results: []TestResult{{Index: 1, Error: errMsg}}}
	}

	return parseGoTestOutput(combined)
}

// parseGoTestOutput converts `go test -v` stdout into a RunResult.
//
// Relevant lines from -v output:
//
//	=== RUN   TestFoo
//	--- PASS: TestFoo (0.00s)
//	--- FAIL: TestFoo (0.00s)
//	    main_test.go:12: expected 30, got 0
//
// We collect each RUN block, then mark it PASS or FAIL from the matching --- line,
// and accumulate any indented lines between them as the failure message.
func parseGoTestOutput(output string) RunResult {
	type block struct {
		name    string
		passed  bool
		lines   []string // failure detail lines
		decided bool     // have we seen the --- PASS/FAIL line?
	}

	var blocks []*block
	byName := map[string]*block{}
	var current *block

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "=== RUN"):
			// "=== RUN   TestFoo" or "=== RUN   TestFoo/subtest"
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				name := parts[2]
				b := &block{name: name}
				blocks = append(blocks, b)
				byName[name] = b
				current = b
			}

		case strings.HasPrefix(line, "--- PASS:") || strings.HasPrefix(line, "--- FAIL:"):
			// "--- PASS: TestFoo (0.00s)"
			passed := strings.HasPrefix(line, "--- PASS:")
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := parts[2]
				if b, ok := byName[name]; ok {
					b.passed = passed
					b.decided = true
					current = nil
				}
			}

		default:
			// Indented failure detail line — belongs to the current block
			if current != nil && !current.decided {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					current.lines = append(current.lines, trimmed)
				}
			}
		}
	}

	// Build results; skip sub-tests (names containing "/") to avoid duplication —
	// the parent test's PASS/FAIL already covers them. Show them only if we have
	// no top-level results at all.
	var results []TestResult
	index := 1
	for _, b := range blocks {
		if strings.Contains(b.name, "/") {
			continue
		}
		tr := TestResult{
			Index:  index,
			Passed: b.passed,
			// Use the test function name as the human-readable label via the Got field
			// so the frontend can display it. Input/Expected stay empty for Go tests.
			Got: b.name,
		}
		if !b.passed && len(b.lines) > 0 {
			tr.Error = strings.Join(b.lines, "\n")
		}
		results = append(results, tr)
		index++
	}

	// If all tests were sub-tests (or none had RUN lines), fall back to showing them.
	if len(results) == 0 {
		for _, b := range blocks {
			tr := TestResult{
				Index:  index,
				Passed: b.passed,
				Got:    b.name,
			}
			if !b.passed && len(b.lines) > 0 {
				tr.Error = strings.Join(b.lines, "\n")
			}
			results = append(results, tr)
			index++
		}
	}

	// If we still have nothing (e.g. build failed but output wasn't empty), surface raw output.
	if len(results) == 0 {
		return RunResult{
			Results: []TestResult{{Index: 1, Error: strings.TrimSpace(output)}},
		}
	}

	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	total := len(results)
	return RunResult{
		Passed:    passed,
		Total:     total,
		AllPassed: passed == total && total > 0,
		Results:   results,
	}
}

// runAgainstTestCases runs code against JSON-defined test cases (Python/JS/bash/legacy Go).
func runAgainstTestCases(language, code string, testCases []TestCase) RunResult {
	result := RunResult{Total: len(testCases)}
	var mu sync.Mutex
	var wg sync.WaitGroup

	results := make([]TestResult, len(testCases))

	for i, tc := range testCases {
		wg.Add(1)
		time.Sleep(time.Duration(i) * 50 * time.Millisecond)
		go func(idx int, tc TestCase) {
			defer wg.Done()
			got, runErr := runCode(language, code, tc.Input)
			tr := TestResult{
				Index:    idx + 1,
				Input:    tc.Input,
				Expected: strings.TrimSpace(tc.Expected),
				Got:      got,
			}
			if runErr != "" {
				tr.Error = runErr
				tr.Passed = false
			} else {
				tr.Passed = strings.TrimSpace(got) == strings.TrimSpace(tc.Expected)
			}
			mu.Lock()
			results[idx] = tr
			mu.Unlock()
		}(i, tc)
	}
	wg.Wait()

	result.Results = results
	for _, r := range results {
		if r.Passed {
			result.Passed++
		}
	}
	result.AllPassed = result.Passed == result.Total && result.Total > 0
	return result
}

func handleRunCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := requireAuth(w, r)
	if u == nil {
		return
	}

	var body struct {
		QuestionID int    `json:"question_id"`
		Code       string `json:"code"`
		Language   string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if body.Code == "" {
		http.Error(w, "code required", 400)
		return
	}
	if body.Language == "" {
		body.Language = "go"
	}

	// Fetch both test_file and test_cases for this question
	var testCasesJSON, testFile string
	err := db.QueryRow(
		`SELECT COALESCE(test_cases,'[]'), COALESCE(test_file,'') FROM questions WHERE id=$1`,
		body.QuestionID,
	).Scan(&testCasesJSON, &testFile)
	if err != nil {
		http.Error(w, "question not found", 404)
		return
	}

	// ── Routing logic ──────────────────────────────────────
	// Priority 1: Go language + test_file present → use go test runner
	if body.Language == "go" && strings.TrimSpace(testFile) != "" {
		result := runGoTest(body.Code, testFile)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	// Priority 2: JSON test cases present → use stdin/stdout runner (all languages)
	var testCases []TestCase
	if err := json.Unmarshal([]byte(testCasesJSON), &testCases); err == nil && len(testCases) > 0 {
		result := runAgainstTestCases(body.Language, body.Code, testCases)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	// Priority 3: No test cases defined — run once with empty input, show raw output
	out, runErr := runCode(body.Language, body.Code, "")
	result := RunResult{Total: 0, Passed: 0, AllPassed: false}
	if runErr != "" {
		result.Results = []TestResult{{Index: 1, Error: runErr, Passed: false}}
	} else {
		result.Results = []TestResult{{
			Index:  1,
			Got:    out,
			Passed: false,
			Error:  "no test cases defined for this question — output shown above",
		}}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ── CORS + ROUTER ─────────────────────────────────────────
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,PATCH,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next(w, r)
	}
}

func main() {
	if err := initDB(); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	defer db.Close()

	http.Handle("/", http.FileServer(http.Dir("static")))

	// Auth
	http.HandleFunc("/api/auth/register", withCORS(handleRegister))
	http.HandleFunc("/api/auth/login", withCORS(handleLogin))
	http.HandleFunc("/api/auth/logout", withCORS(handleLogout))
	http.HandleFunc("/api/me", withCORS(handleMe))

	// Admin
	http.HandleFunc("/api/admin/users", withCORS(handleAdminUsers))
	http.HandleFunc("/api/admin/promote", withCORS(handleAdminPromote))

	// Questions
	http.HandleFunc("/api/questions", withCORS(handleQuestions))
	http.HandleFunc("/api/questions/delete", withCORS(handleDeleteQuestion))
	http.HandleFunc("/api/questions/visibility", withCORS(handleQuestionVisibility))

	// Submissions
	http.HandleFunc("/api/submissions", withCORS(handleSubmissions))
	http.HandleFunc("/api/submissions/delete", withCORS(handleDeleteSubmission))

	// Reviews
	http.HandleFunc("/api/reviews", withCORS(handleReviews))

	// Leaderboard
	http.HandleFunc("/api/leaderboard", withCORS(handleLeaderboard))

	// Code execution
	http.HandleFunc("/api/run", withCORS(handleRunCode))

	// Social
	http.HandleFunc("/api/search", withCORS(handleSearch))
	http.HandleFunc("/api/follows", withCORS(handleFollows))
	http.HandleFunc("/api/notifications", withCORS(handleNotifications))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Server running at http://localhost:%s", port)
	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
