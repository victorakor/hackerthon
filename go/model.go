package app

import "time"

// ── Existing models ───────────────────────────────────────────────────────────

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
	TestCases   string `json:"test_cases"`
	TestFile    string `json:"test_file"`
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

// ── Challenge models ──────────────────────────────────────────────────────────

// Challenge represents a 1v1 coding contest between two users.
type Challenge struct {
	ID              int       `json:"id"`
	ChallengerID    int       `json:"challenger_id"`
	OpponentID      int       `json:"opponent_id"`
	ScheduledAt     time.Time `json:"scheduled_at"`
	Status          string    `json:"status"`    // pending|accepted|rejected|active|completed|cancelled
	WinnerID        int       `json:"winner_id"` // 0 = tie or not yet decided
	ChallengerScore int       `json:"challenger_score"`
	OpponentScore   int       `json:"opponent_score"`
	CreatedAt       time.Time `json:"created_at"`
}

// ChallengeDetail is Challenge + display names, returned to the frontend.
type ChallengeDetail struct {
	Challenge
	ChallengerName string `json:"challenger_name"`
	OpponentName   string `json:"opponent_name"`
	WinnerName     string `json:"winner_name,omitempty"`
	// QuestionIDs is populated only when the challenge is active/completed.
	QuestionIDs []int `json:"question_ids,omitempty"`
}

// ChallengeResult holds the final outcome of a completed challenge.
type ChallengeResult struct {
	ChallengeID     int    `json:"challenge_id"`
	Status          string `json:"status"`
	ChallengerID    int    `json:"challenger_id"`
	ChallengerName  string `json:"challenger_name"`
	ChallengerScore int    `json:"challenger_score"`
	OpponentID      int    `json:"opponent_id"`
	OpponentName    string `json:"opponent_name"`
	OpponentScore   int    `json:"opponent_score"`
	WinnerID        int    `json:"winner_id"` // 0 = tie
	WinnerName      string `json:"winner_name,omitempty"`
}

// ── Tournament models ─────────────────────────────────────────────────────────

// Tournament represents an admin-created multi-user coding contest.
type Tournament struct {
	ID              int       `json:"id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	CreatedBy       int       `json:"created_by"`
	ScheduledAt     time.Time `json:"scheduled_at"`
	MaxParticipants int       `json:"max_participants"`
	Status          string    `json:"status"` // upcoming|active|completed
	CreatedAt       time.Time `json:"created_at"`
}

// TournamentDetail is Tournament + live participant count + questions when active.
type TournamentDetail struct {
	Tournament
	ParticipantCount int   `json:"participant_count"`
	IsJoined         bool  `json:"is_joined"`
	QuestionIDs      []int `json:"question_ids,omitempty"`
}

// TournamentRank is one row in the post-tournament leaderboard.
// TournamentRank is one row in the post-tournament leaderboard.
type TournamentRank struct {
	Rank         int    `json:"rank"`
	UserID       int    `json:"user_id"`
	Name         string `json:"name"`
	Score        int    `json:"score"` // number of questions passed
	Disqualified bool   `json:"disqualified"`
}

// ── Notification model ────────────────────────────────────────────────────────

// Notification is a lightweight event pushed to a user.
// kind values: challenge_received | challenge_accepted | challenge_rejected |
//
//	contest_start | contest_end | tournament_start | tournament_end
type Notification struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Kind      string    `json:"kind"`
	Payload   string    `json:"payload"` // JSON string, parsed by the frontend
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}
