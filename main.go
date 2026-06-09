package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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

	// Add visible column if it doesn't exist (migration for existing DBs)
	db.Exec(`ALTER TABLE questions ADD COLUMN IF NOT EXISTS visible BOOLEAN NOT NULL DEFAULT FALSE`)
	// Add user_id columns if they don't exist
	db.Exec(`ALTER TABLE submissions ADD COLUMN IF NOT EXISTS user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`)
	db.Exec(`ALTER TABLE reviews ADD COLUMN IF NOT EXISTS user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`)

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
		_, err := db.Exec(`INSERT INTO questions (title, description, difficulty, category, hint_url, hint_text, visible) VALUES ($1,$2,$3,$4,$5,$6,FALSE)`,
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
	// Count unique questions answered
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
	admin := requireAdmin(w, r)
	if admin == nil {
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
	// ← ADD THIS
	if body.ID == admin.ID && !body.IsAdmin {
		http.Error(w, "cannot remove your own admin status", 400)
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
			// Admin sees all questions
			rows, err = db.Query(`SELECT id,title,description,difficulty,category,hint_url,hint_text,visible FROM questions ORDER BY id`)
		} else {
			// Normal users only see visible questions
			rows, err = db.Query(`SELECT id,title,description,difficulty,category,hint_url,hint_text,visible FROM questions WHERE visible=TRUE ORDER BY id`)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var qs []Question
		for rows.Next() {
			var q Question
			rows.Scan(&q.ID, &q.Title, &q.Description, &q.Difficulty, &q.Category, &q.HintURL, &q.HintText, &q.Visible)
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
		err := db.QueryRow(`INSERT INTO questions (title,description,difficulty,category,hint_url,hint_text,visible) VALUES ($1,$2,$3,$4,$5,$6,FALSE) RETURNING id`,
			q.Title, q.Description, q.Difficulty, q.Category, q.HintURL, q.HintText).Scan(&q.ID)
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

	// Define UserProfile struct locally if not available globally
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

	var results []UserProfile = []UserProfile{} // Initialize as correct type

	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin, &u.CreatedAt); err != nil {
			continue
		}

		// Use a helper function to process submissions so we can close rows immediately
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
			defer subRows.Close() // Properly closes connection after this user is processed

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
		WriteTimeout: 15 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
