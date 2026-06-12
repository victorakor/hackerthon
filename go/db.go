package app

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var DB *sql.DB

func InitDB() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL environment variable not set")
	}
	var err error
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("opening db: %w", err)
	}
	DB.SetMaxOpenConns(10)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(5 * time.Minute)
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("pinging db: %w", err)
	}
	return createTables()
}

func createTables() error {
	// ── Core tables ───────────────────────────────────────────────────────────
	_, err := DB.Exec(`
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
		return fmt.Errorf("creating core tables: %w", err)
	}

	// ── Challenge tables ──────────────────────────────────────────────────────
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS challenges (
			id             SERIAL PRIMARY KEY,
			challenger_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			opponent_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			scheduled_at   TIMESTAMPTZ NOT NULL,
			status         TEXT NOT NULL DEFAULT 'pending',
			winner_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,
			challenger_score INTEGER NOT NULL DEFAULT 0,
			opponent_score   INTEGER NOT NULL DEFAULT 0,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT challenges_status_check
				CHECK (status IN ('pending','accepted','rejected','active','completed','cancelled'))
		);

		CREATE TABLE IF NOT EXISTS challenge_questions (
			challenge_id INTEGER NOT NULL REFERENCES challenges(id) ON DELETE CASCADE,
			question_id  INTEGER NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
			sort_order   INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (challenge_id, question_id)
		);

		CREATE TABLE IF NOT EXISTS challenge_submissions (
			id           SERIAL PRIMARY KEY,
			challenge_id INTEGER NOT NULL REFERENCES challenges(id) ON DELETE CASCADE,
			user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			question_id  INTEGER NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
			passed       BOOLEAN NOT NULL DEFAULT FALSE,
			submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (challenge_id, user_id, question_id)
		);
	`)
	if err != nil {
		return fmt.Errorf("creating challenge tables: %w", err)
	}

	// ── Tournament tables ─────────────────────────────────────────────────────
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS tournaments (
			id               SERIAL PRIMARY KEY,
			title            TEXT NOT NULL,
			description      TEXT NOT NULL DEFAULT '',
			created_by       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			scheduled_at     TIMESTAMPTZ NOT NULL,
			max_participants INTEGER NOT NULL DEFAULT 10,
			status           TEXT NOT NULL DEFAULT 'upcoming',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT tournaments_status_check
				CHECK (status IN ('upcoming','active','completed'))
		);

		CREATE TABLE IF NOT EXISTS tournament_questions (
			tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
			question_id   INTEGER NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
			sort_order    INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (tournament_id, question_id)
		);

		CREATE TABLE IF NOT EXISTS tournament_participants (
			tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
			user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			joined_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (tournament_id, user_id)
		);

		CREATE TABLE IF NOT EXISTS tournament_submissions (
			id            SERIAL PRIMARY KEY,
			tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
			user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			question_id   INTEGER NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
			passed        BOOLEAN NOT NULL DEFAULT FALSE,
			submitted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tournament_id, user_id, question_id)
		);
	`)
	if err != nil {
		return fmt.Errorf("creating tournament tables: %w", err)
	}

	// ── Notifications table ───────────────────────────────────────────────────
	// Used to push challenge invites and contest-start events to users.
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS notifications (
			id         SERIAL PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			kind       TEXT NOT NULL,
			payload    TEXT NOT NULL DEFAULT '{}',
			read       BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("creating notifications table: %w", err)
	}

	// ── Migrations for existing DBs ───────────────────────────────────────────
	DB.Exec(`ALTER TABLE questions ADD COLUMN IF NOT EXISTS visible BOOLEAN NOT NULL DEFAULT FALSE`)
	DB.Exec(`ALTER TABLE submissions ADD COLUMN IF NOT EXISTS user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`)
	DB.Exec(`ALTER TABLE reviews ADD COLUMN IF NOT EXISTS user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`)
	DB.Exec(`ALTER TABLE questions ADD COLUMN IF NOT EXISTS test_cases TEXT NOT NULL DEFAULT '[]'`)
	DB.Exec(`ALTER TABLE questions ADD COLUMN IF NOT EXISTS test_file TEXT NOT NULL DEFAULT ''`)
	
	// Challenge/tournament columns added in v2
	DB.Exec(`ALTER TABLE challenges ADD COLUMN IF NOT EXISTS challenger_score INTEGER NOT NULL DEFAULT 0`)
	DB.Exec(`ALTER TABLE challenges ADD COLUMN IF NOT EXISTS opponent_score INTEGER NOT NULL DEFAULT 0`)

	// Anti-cheat columns added in v3
	DB.Exec(`ALTER TABLE tournament_participants ADD COLUMN IF NOT EXISTS violation_count INTEGER NOT NULL DEFAULT 0`)
	DB.Exec(`ALTER TABLE tournament_participants ADD COLUMN IF NOT EXISTS disqualified BOOLEAN NOT NULL DEFAULT FALSE`)
	DB.Exec(`ALTER TABLE tournament_participants ADD COLUMN IF NOT EXISTS disqualified_at TIMESTAMPTZ`)
	DB.Exec(`ALTER TABLE challenges ADD COLUMN IF NOT EXISTS challenger_violations INTEGER NOT NULL DEFAULT 0`)
	DB.Exec(`ALTER TABLE challenges ADD COLUMN IF NOT EXISTS opponent_violations INTEGER NOT NULL DEFAULT 0`)
	DB.Exec(`ALTER TABLE challenges ADD COLUMN IF NOT EXISTS challenger_disqualified BOOLEAN NOT NULL DEFAULT FALSE`)
	DB.Exec(`ALTER TABLE challenges ADD COLUMN IF NOT EXISTS opponent_disqualified BOOLEAN NOT NULL DEFAULT FALSE`)

	if err := SeedAdminUser(); err != nil {
		log.Printf("admin seed: %v", err)
	}

	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM questions`).Scan(&count)
	if count == 0 {
		return SeedQuestions()
	}
	return nil
}

// ── Notification helpers ──────────────────────────────────────────────────────

// CreateNotification inserts a notification row and returns its id.
func CreateNotification(userID int, kind, payload string) (int, error) {
	var id int
	err := DB.QueryRow(
		`INSERT INTO notifications (user_id, kind, payload) VALUES ($1,$2,$3) RETURNING id`,
		userID, kind, payload,
	).Scan(&id)
	return id, err
}

// ── Challenge DB helpers ──────────────────────────────────────────────────────

// GetChallenge fetches a single challenge by id.
func GetChallenge(id int) (*Challenge, error) {
	c := &Challenge{}
	err := DB.QueryRow(`
		SELECT id, challenger_id, opponent_id, scheduled_at, status,
		       COALESCE(winner_id, 0), challenger_score, opponent_score, created_at
		FROM challenges WHERE id = $1`, id,
	).Scan(&c.ID, &c.ChallengerID, &c.OpponentID, &c.ScheduledAt, &c.Status,
		&c.WinnerID, &c.ChallengerScore, &c.OpponentScore, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// GetChallengeQuestions returns the ordered question IDs for a challenge.
func GetChallengeQuestions(challengeID int) ([]int, error) {
	rows, err := DB.Query(
		`SELECT question_id FROM challenge_questions WHERE challenge_id=$1 ORDER BY sort_order`,
		challengeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}

// CountChallengeScore counts how many questions a user passed in a challenge.
func CountChallengeScore(challengeID, userID int) (int, error) {
	var n int
	err := DB.QueryRow(
		`SELECT COUNT(*) FROM challenge_submissions
		 WHERE challenge_id=$1 AND user_id=$2 AND passed=TRUE`,
		challengeID, userID,
	).Scan(&n)
	return n, err
}

// ── Tournament DB helpers ─────────────────────────────────────────────────────

// GetTournament fetches a single tournament by id.
func GetTournament(id int) (*Tournament, error) {
	t := &Tournament{}
	err := DB.QueryRow(`
		SELECT id, title, description, created_by, scheduled_at,
		       max_participants, status, created_at
		FROM tournaments WHERE id = $1`, id,
	).Scan(&t.ID, &t.Title, &t.Description, &t.CreatedBy,
		&t.ScheduledAt, &t.MaxParticipants, &t.Status, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTournamentParticipantCount returns the current number of joined participants.
func GetTournamentParticipantCount(tournamentID int) (int, error) {
	var n int
	err := DB.QueryRow(
		`SELECT COUNT(*) FROM tournament_participants WHERE tournament_id=$1`,
		tournamentID,
	).Scan(&n)
	return n, err
}

// GetTournamentQuestions returns the ordered question IDs for a tournament.
func GetTournamentQuestions(tournamentID int) ([]int, error) {
	rows, err := DB.Query(
		`SELECT question_id FROM tournament_questions WHERE tournament_id=$1 ORDER BY sort_order`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}

// GetTournamentLeaderboard returns participants ranked by questions passed.
func GetTournamentLeaderboard(tournamentID int) ([]TournamentRank, error) {
	rows, err := DB.Query(`
		SELECT u.id, u.name,
		       COUNT(ts.id) FILTER (WHERE ts.passed = TRUE) AS score,
		       COALESCE(tp.disqualified, FALSE) AS disqualified
		FROM tournament_participants tp
		JOIN users u ON u.id = tp.user_id
		LEFT JOIN tournament_submissions ts
		       ON ts.tournament_id = tp.tournament_id AND ts.user_id = tp.user_id
		WHERE tp.tournament_id = $1
		GROUP BY u.id, u.name, tp.disqualified
		ORDER BY tp.disqualified ASC, score DESC, u.name ASC`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ranks []TournamentRank
	pos := 1
	for rows.Next() {
		var r TournamentRank
		rows.Scan(&r.UserID, &r.Name, &r.Score, &r.Disqualified)
		r.Rank = pos
		pos++
		ranks = append(ranks, r)
	}
	return ranks, nil
}

// GetChallengeLeaderboard returns both participants' scores for a challenge.
func GetChallengeResult(challengeID int) (*ChallengeResult, error) {
	c, err := GetChallenge(challengeID)
	if err != nil {
		return nil, err
	}

	var challengerName, opponentName string
	DB.QueryRow(`SELECT name FROM users WHERE id=$1`, c.ChallengerID).Scan(&challengerName)
	DB.QueryRow(`SELECT name FROM users WHERE id=$1`, c.OpponentID).Scan(&opponentName)

	var winnerName string
	if c.WinnerID != 0 {
		DB.QueryRow(`SELECT name FROM users WHERE id=$1`, c.WinnerID).Scan(&winnerName)
	}

	return &ChallengeResult{
		ChallengeID:     c.ID,
		Status:          c.Status,
		ChallengerID:    c.ChallengerID,
		ChallengerName:  challengerName,
		ChallengerScore: c.ChallengerScore,
		OpponentID:      c.OpponentID,
		OpponentName:    opponentName,
		OpponentScore:   c.OpponentScore,
		WinnerID:        c.WinnerID,
		WinnerName:      winnerName,
	}, nil
}

// ── Seed functions ────────────────────────────────────────────────────────────

func SeedAdminUser() error {
	var exists bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, "victorakor04@gmail.com").Scan(&exists)
	if exists {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("VicTorAKor3"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`INSERT INTO users (name, email, password_hash, is_admin) VALUES ($1,$2,$3,$4)`,
		"Victor", "victorakor04@gmail.com", string(hash), true)
	if err != nil {
		return fmt.Errorf("seeding admin: %w", err)
	}
	log.Println("✓ Admin user seeded")
	return nil
}

func SeedQuestions() error {
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
		_, err := DB.Exec(
			`INSERT INTO questions (title,description,difficulty,category,hint_url,hint_text,visible,test_cases,test_file) VALUES ($1,$2,$3,$4,$5,$6,FALSE,'[]','')`,
			q.Title, q.Description, q.Difficulty, q.Category, q.HintURL, q.HintText)
		if err != nil {
			return fmt.Errorf("seeding %q: %w", q.Title, err)
		}
	}
	log.Printf("✓ Seeded %d questions", len(seeds))
	return nil
}

// HasContestOverlap returns true if the given user already has an accepted challenge
// or joined upcoming/active tournament whose 1-hour window overlaps [start, start+1h).
func HasContestOverlap(userID int, start time.Time) (bool, error) {
	end := start.Add(time.Hour)

	// Check challenges: status accepted or active, window overlaps
	var challengeConflict bool
	err := DB.QueryRow(`
        SELECT EXISTS(
            SELECT 1 FROM challenges
            WHERE status IN ('accepted', 'active')
            AND (challenger_id = $1 OR opponent_id = $1)
            AND scheduled_at < $3
            AND scheduled_at + INTERVAL '1 hour' > $2
        )`, userID, start, end).Scan(&challengeConflict)
	if err != nil {
		return false, err
	}
	if challengeConflict {
		return true, nil
	}

	// Check tournaments: user is joined and tournament is upcoming or active, window overlaps
	var tournamentConflict bool
	err = DB.QueryRow(`
        SELECT EXISTS(
            SELECT 1 FROM tournaments t
            JOIN tournament_participants tp ON tp.tournament_id = t.id
            WHERE tp.user_id = $1
            AND t.status IN ('upcoming', 'active')
            AND t.scheduled_at < $3
            AND t.scheduled_at + INTERVAL '1 hour' > $2
        )`, userID, start, end).Scan(&tournamentConflict)
	if err != nil {
		return false, err
	}
	return tournamentConflict, nil
}
