package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type Question struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Difficulty  string `json:"difficulty"`
	Category    string `json:"category"`
	HintURL     string `json:"hint_url"`
	HintText    string `json:"hint_text"`
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

	return createTable()
}

func createTable() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS questions (
			id          SERIAL PRIMARY KEY,
			title       TEXT NOT NULL,
			description TEXT NOT NULL,
			difficulty  TEXT NOT NULL DEFAULT 'medium',
			category    TEXT NOT NULL DEFAULT 'General',
			hint_url    TEXT NOT NULL DEFAULT 'https://pkg.go.dev',
			hint_text   TEXT NOT NULL DEFAULT 'Go documentation'
		)
	`)
	if err != nil {
		return fmt.Errorf("creating table: %w", err)
	}

	// Seed only if empty
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM questions`).Scan(&count)
	if count == 0 {
		return seedQuestions()
	}
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
		_, err := db.Exec(`
			INSERT INTO questions (title, description, difficulty, category, hint_url, hint_text)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, q.Title, q.Description, q.Difficulty, q.Category, q.HintURL, q.HintText)
		if err != nil {
			return fmt.Errorf("seeding question %q: %w", q.Title, err)
		}
	}
	log.Printf("✓ Seeded %d questions", len(seeds))
	return nil
}

// ── HANDLERS ──────────────────────────────────────────────
func handleQuestions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, title, description, difficulty, category, hint_url, hint_text FROM questions ORDER BY id`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var questions []Question
		for rows.Next() {
			var q Question
			if err := rows.Scan(&q.ID, &q.Title, &q.Description, &q.Difficulty, &q.Category, &q.HintURL, &q.HintText); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			questions = append(questions, q)
		}
		if questions == nil {
			questions = []Question{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(questions)

	case http.MethodPost:
		var q Question
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if q.Title == "" || q.Description == "" {
			http.Error(w, "title and description are required", http.StatusBadRequest)
			return
		}
		if q.Difficulty == "" { q.Difficulty = "medium" }
		if q.Category == ""   { q.Category = "General" }
		if q.HintURL == ""    { q.HintURL = "https://pkg.go.dev" }
		if q.HintText == ""   { q.HintText = "Go documentation" }

		err := db.QueryRow(`
			INSERT INTO questions (title, description, difficulty, category, hint_url, hint_text)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, q.Title, q.Description, q.Difficulty, q.Category, q.HintURL, q.HintText).Scan(&q.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(q)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleDeleteQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	result, err := db.Exec(`DELETE FROM questions WHERE id = $1`, payload.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		http.Error(w, "question not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
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
	http.HandleFunc("/api/questions", withCORS(handleQuestions))
	http.HandleFunc("/api/questions/delete", withCORS(handleDeleteQuestion))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Server running at http://localhost:%s", port)
	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
