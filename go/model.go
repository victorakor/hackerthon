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

type Challenge struct {
	ID                   int        `json:"id"`
	ChallengerID         int        `json:"challenger_id"`
	OpponentID           int        `json:"opponent_id"`
	ScheduledAt          time.Time  `json:"scheduled_at"`
	Status               string     `json:"status"`
	WinnerID             int        `json:"winner_id"`
	ChallengerScore      int        `json:"challenger_score"`
	OpponentScore        int        `json:"opponent_score"`
	DurationMinutes      int        `json:"duration_minutes"`
	ChallengerEnteredAt  *time.Time `json:"challenger_entered_at,omitempty"`
	OpponentEnteredAt    *time.Time `json:"opponent_entered_at,omitempty"`
	ChallengerFinishedAt *time.Time `json:"challenger_finished_at,omitempty"`
	OpponentFinishedAt   *time.Time `json:"opponent_finished_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

type ChallengeDetail struct {
	Challenge
	ChallengerName string `json:"challenger_name"`
	OpponentName   string `json:"opponent_name"`
	WinnerName     string `json:"winner_name,omitempty"`
	QuestionIDs    []int  `json:"question_ids,omitempty"`
}

type ChallengeResult struct {
	ChallengeID     int    `json:"challenge_id"`
	Status          string `json:"status"`
	ChallengerID    int    `json:"challenger_id"`
	ChallengerName  string `json:"challenger_name"`
	ChallengerScore int    `json:"challenger_score"`
	OpponentID      int    `json:"opponent_id"`
	OpponentName    string `json:"opponent_name"`
	OpponentScore   int    `json:"opponent_score"`
	WinnerID        int    `json:"winner_id"`
	WinnerName      string `json:"winner_name,omitempty"`
}

// ArenaEnterResponse is returned by /enter for both challenges and tournaments.
type ArenaEnterResponse struct {
	EndsAt          string     `json:"ends_at"`
	DurationMinutes int        `json:"duration_minutes"`
	Questions       []Question `json:"questions"`
}

// ArenaSubmitResponse is returned by /arena-submit.
type ArenaSubmitResponse struct {
	RunResult    RunResult `json:"run_result"`
	Passed       bool      `json:"passed"`
	Finished     bool      `json:"finished"`
	Disqualified bool      `json:"disqualified"`
}

// ── Tournament models ─────────────────────────────────────────────────────────

type Tournament struct {
	ID              int       `json:"id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	CreatedBy       int       `json:"created_by"`
	ScheduledAt     time.Time `json:"scheduled_at"`
	MaxParticipants int       `json:"max_participants"`
	Status          string    `json:"status"`
	DurationMinutes int       `json:"duration_minutes"`
	CreatedAt       time.Time `json:"created_at"`
}

type TournamentDetail struct {
	Tournament
	ParticipantCount int   `json:"participant_count"`
	IsJoined         bool  `json:"is_joined"`
	QuestionIDs      []int `json:"question_ids,omitempty"`
}

type TournamentRank struct {
	Rank         int    `json:"rank"`
	UserID       int    `json:"user_id"`
	Name         string `json:"name"`
	Score        int    `json:"score"`
	Disqualified bool   `json:"disqualified"`
}

type Notification struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Kind      string    `json:"kind"`
	Payload   string    `json:"payload"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}
