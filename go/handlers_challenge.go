package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// POST /api/challenges
func HandleCreateChallenge(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	var body struct {
		OpponentID  int    `json:"opponent_id"`
		ScheduledAt string `json:"scheduled_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if body.OpponentID == 0 {
		http.Error(w, "opponent_id required", 400)
		return
	}
	if body.OpponentID == u.ID {
		http.Error(w, "cannot challenge yourself", 400)
		return
	}

	scheduledAt, err := time.Parse(time.RFC3339, body.ScheduledAt)
	if err != nil {
		http.Error(w, "scheduled_at must be RFC3339 (e.g. 2026-06-13T18:00:00Z)", 400)
		return
	}
	if scheduledAt.Before(time.Now().UTC().Add(5 * time.Minute)) {
		http.Error(w, "scheduled_at must be at least 5 minutes in the future", 400)
		return
	}

	var exists bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, body.OpponentID).Scan(&exists)
	if !exists {
		http.Error(w, "opponent not found", 404)
		return
	}

	var conflict bool
	DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM challenges
			WHERE status IN ('pending','accepted','active')
			AND (
				(challenger_id=$1 AND opponent_id=$2) OR
				(challenger_id=$2 AND opponent_id=$1)
			)
		)`, u.ID, body.OpponentID).Scan(&conflict)
	if conflict {
		http.Error(w, "an active or pending challenge already exists between you two", 409)
		return
	}

	if overlap, err := HasContestOverlap(u.ID, scheduledAt); err != nil || overlap {
		http.Error(w, "you already have a contest scheduled at that time", 409)
		return
	}
	if overlap, err := HasContestOverlap(body.OpponentID, scheduledAt); err != nil || overlap {
		http.Error(w, "your opponent already has a contest scheduled at that time", 409)
		return
	}

	var id int
	err = DB.QueryRow(`
		INSERT INTO challenges (challenger_id, opponent_id, scheduled_at)
		VALUES ($1,$2,$3) RETURNING id`,
		u.ID, body.OpponentID, scheduledAt.UTC(),
	).Scan(&id)
	if err != nil {
		http.Error(w, "failed to create challenge", 500)
		return
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"challenge_id":    id,
		"challenger_id":   u.ID,
		"challenger_name": u.Name,
		"scheduled_at":    scheduledAt.UTC().Format(time.RFC3339),
	})
	CreateNotification(body.OpponentID, "challenge_received", string(payload))

	adminRows, _ := DB.Query(`SELECT id FROM users WHERE is_admin=TRUE`)
	if adminRows != nil {
		defer adminRows.Close()
		adminPayload, _ := json.Marshal(map[string]interface{}{
			"challenge_id": id,
			"message":      fmt.Sprintf("New challenge #%d created, awaiting question assignment", id),
		})
		for adminRows.Next() {
			var aid int
			adminRows.Scan(&aid)
			CreateNotification(aid, "challenge_pending_questions", string(adminPayload))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      id,
		"message": "Challenge sent",
	})
}

// PATCH /api/challenges/{id}/accept
func HandleAcceptChallenge(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/challenges/", "/accept")
	if id == 0 {
		http.Error(w, "invalid challenge id", 400)
		return
	}

	c, err := GetChallenge(id)
	if err != nil {
		http.Error(w, "challenge not found", 404)
		return
	}
	if c.OpponentID != u.ID {
		http.Error(w, "only the opponent can accept", 403)
		return
	}
	if c.Status != "pending" {
		http.Error(w, "challenge is not pending", 409)
		return
	}

	if overlap, err := HasContestOverlap(u.ID, c.ScheduledAt); err != nil || overlap {
		http.Error(w, "you already have a contest at that time", 409)
		return
	}
	if overlap, err := HasContestOverlap(c.ChallengerID, c.ScheduledAt); err != nil || overlap {
		http.Error(w, "the challenger already has a contest at that time", 409)
		return
	}

	_, err = DB.Exec(`UPDATE challenges SET status='accepted' WHERE id=$1`, id)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"challenge_id":  id,
		"opponent_name": u.Name,
		"scheduled_at":  c.ScheduledAt.UTC().Format(time.RFC3339),
	})
	CreateNotification(c.ChallengerID, "challenge_accepted", string(payload))

	adminRows, _ := DB.Query(`SELECT id FROM users WHERE is_admin=TRUE`)
	if adminRows != nil {
		defer adminRows.Close()
		adminPayload, _ := json.Marshal(map[string]interface{}{
			"challenge_id": id,
			"message":      fmt.Sprintf("Challenge #%d accepted — please assign questions", id),
		})
		for adminRows.Next() {
			var aid int
			adminRows.Scan(&aid)
			CreateNotification(aid, "challenge_needs_questions", string(adminPayload))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// PATCH /api/challenges/{id}/reject
func HandleRejectChallenge(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/challenges/", "/reject")
	if id == 0 {
		http.Error(w, "invalid challenge id", 400)
		return
	}

	c, err := GetChallenge(id)
	if err != nil {
		http.Error(w, "challenge not found", 404)
		return
	}
	if c.OpponentID != u.ID {
		http.Error(w, "only the opponent can reject", 403)
		return
	}
	if c.Status != "pending" {
		http.Error(w, "challenge is not pending", 409)
		return
	}

	DB.Exec(`UPDATE challenges SET status='rejected' WHERE id=$1`, id)

	payload, _ := json.Marshal(map[string]interface{}{
		"challenge_id":  id,
		"opponent_name": u.Name,
	})
	CreateNotification(c.ChallengerID, "challenge_rejected", string(payload))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
}

// GET /api/challenges
func HandleListChallenges(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	rows, err := DB.Query(`
		SELECT c.id, c.challenger_id, c.opponent_id, c.scheduled_at, c.status,
		       COALESCE(c.winner_id, 0), c.challenger_score, c.opponent_score,
		       COALESCE(c.duration_minutes,60),
		       c.challenger_entered_at, c.opponent_entered_at,
		       c.challenger_finished_at, c.opponent_finished_at,
		       c.created_at,
		       uc.name AS challenger_name, uo.name AS opponent_name
		FROM challenges c
		JOIN users uc ON uc.id = c.challenger_id
		JOIN users uo ON uo.id = c.opponent_id
		WHERE c.challenger_id=$1 OR c.opponent_id=$1
		ORDER BY c.created_at DESC`, u.ID)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	defer rows.Close()

	var out []ChallengeDetail
	for rows.Next() {
		var d ChallengeDetail
		rows.Scan(
			&d.ID, &d.ChallengerID, &d.OpponentID, &d.ScheduledAt, &d.Status,
			&d.WinnerID, &d.ChallengerScore, &d.OpponentScore,
			&d.DurationMinutes,
			&d.ChallengerEnteredAt, &d.OpponentEnteredAt,
			&d.ChallengerFinishedAt, &d.OpponentFinishedAt,
			&d.CreatedAt,
			&d.ChallengerName, &d.OpponentName,
		)
		out = append(out, d)
	}
	if out == nil {
		out = []ChallengeDetail{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// GET /api/challenges/{id}
func HandleGetChallenge(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/challenges/", "")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	c, err := GetChallenge(id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}

	if c.ChallengerID != u.ID && c.OpponentID != u.ID && !u.IsAdmin {
		http.Error(w, "forbidden", 403)
		return
	}

	var challengerName, opponentName, winnerName string
	DB.QueryRow(`SELECT name FROM users WHERE id=$1`, c.ChallengerID).Scan(&challengerName)
	DB.QueryRow(`SELECT name FROM users WHERE id=$1`, c.OpponentID).Scan(&opponentName)
	if c.WinnerID != 0 {
		DB.QueryRow(`SELECT name FROM users WHERE id=$1`, c.WinnerID).Scan(&winnerName)
	}

	detail := ChallengeDetail{
		Challenge:      *c,
		ChallengerName: challengerName,
		OpponentName:   opponentName,
		WinnerName:     winnerName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// POST /api/admin/challenges/{id}/questions
// Body: { "question_ids": [1,4,7], "duration_minutes": 45 }
func HandleAssignChallengeQuestions(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "admin only", 403)
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/admin/challenges/", "/questions")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	var body struct {
		QuestionIDs     []int `json:"question_ids"`
		DurationMinutes int   `json:"duration_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.QuestionIDs) == 0 {
		http.Error(w, "question_ids required", 400)
		return
	}
	if body.DurationMinutes <= 0 {
		body.DurationMinutes = 60
	}

	DB.Exec(`UPDATE challenges SET duration_minutes=$1 WHERE id=$2`, body.DurationMinutes, id)
	DB.Exec(`DELETE FROM challenge_questions WHERE challenge_id=$1`, id)
	for i, qid := range body.QuestionIDs {
		DB.Exec(`INSERT INTO challenge_questions (challenge_id, question_id, sort_order) VALUES ($1,$2,$3)
			     ON CONFLICT DO NOTHING`, id, qid, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /api/challenges/{id}/enter
// Sets entered_at for this user and returns questions + ends_at.
func HandleChallengeEnter(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/challenges/", "/enter")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	c, err := GetChallenge(id)
	if err != nil {
		http.Error(w, "challenge not found", 404)
		return
	}
	if c.Status != "active" {
		http.Error(w, "challenge is not active yet", 409)
		return
	}
	if c.ChallengerID != u.ID && c.OpponentID != u.ID {
		http.Error(w, "you are not a participant", 403)
		return
	}

	isChallenger := c.ChallengerID == u.ID

	// Check not disqualified
	var disqualified bool
	if isChallenger {
		DB.QueryRow(`SELECT COALESCE(challenger_disqualified,FALSE) FROM challenges WHERE id=$1`, id).Scan(&disqualified)
	} else {
		DB.QueryRow(`SELECT COALESCE(opponent_disqualified,FALSE) FROM challenges WHERE id=$1`, id).Scan(&disqualified)
	}
	if disqualified {
		http.Error(w, "you have been disqualified from this challenge", 403)
		return
	}

	// Check not already finished
	var finished bool
	if isChallenger {
		DB.QueryRow(`SELECT challenger_finished_at IS NOT NULL FROM challenges WHERE id=$1`, id).Scan(&finished)
	} else {
		DB.QueryRow(`SELECT opponent_finished_at IS NOT NULL FROM challenges WHERE id=$1`, id).Scan(&finished)
	}
	if finished {
		http.Error(w, "you have already completed this challenge", 409)
		return
	}

	// Check time window
	endsAt := EndsAt(c.ScheduledAt, c.DurationMinutes)
	if time.Now().UTC().After(endsAt) {
		http.Error(w, "challenge time has elapsed", 409)
		return
	}

	// Set entered_at (idempotent — only set once)
	if isChallenger {
		DB.Exec(`UPDATE challenges SET challenger_entered_at=NOW() WHERE id=$1 AND challenger_entered_at IS NULL`, id)
	} else {
		DB.Exec(`UPDATE challenges SET opponent_entered_at=NOW() WHERE id=$1 AND opponent_entered_at IS NULL`, id)
	}

	// Fetch full question objects
	qs, err := GetChallengeQuestionObjects(id)
	if err != nil || len(qs) == 0 {
		http.Error(w, "no questions assigned to this challenge yet", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ArenaEnterResponse{
		EndsAt:          endsAt.UTC().Format(time.RFC3339),
		DurationMinutes: c.DurationMinutes,
		Questions:       qs,
	})
}

// POST /api/challenges/{id}/arena-submit
// Server runs tests, records result in challenge_submissions, checks for completion.
// Body: { "question_id": 3, "code": "...", "language": "go" }
func HandleChallengeArenaSubmit(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/challenges/", "/arena-submit")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	c, err := GetChallenge(id)
	if err != nil {
		http.Error(w, "challenge not found", 404)
		return
	}
	if c.Status != "active" {
		http.Error(w, "challenge is not active", 409)
		return
	}
	if c.ChallengerID != u.ID && c.OpponentID != u.ID {
		http.Error(w, "you are not a participant", 403)
		return
	}

	isChallenger := c.ChallengerID == u.ID

	var disqualified bool
	if isChallenger {
		DB.QueryRow(`SELECT COALESCE(challenger_disqualified,FALSE) FROM challenges WHERE id=$1`, id).Scan(&disqualified)
	} else {
		DB.QueryRow(`SELECT COALESCE(opponent_disqualified,FALSE) FROM challenges WHERE id=$1`, id).Scan(&disqualified)
	}
	if disqualified {
		http.Error(w, "you have been disqualified", 403)
		return
	}

	var finished bool
	if isChallenger {
		DB.QueryRow(`SELECT challenger_finished_at IS NOT NULL FROM challenges WHERE id=$1`, id).Scan(&finished)
	} else {
		DB.QueryRow(`SELECT opponent_finished_at IS NOT NULL FROM challenges WHERE id=$1`, id).Scan(&finished)
	}
	if finished {
		http.Error(w, "you have already completed this challenge", 409)
		return
	}

	if time.Now().UTC().After(EndsAt(c.ScheduledAt, c.DurationMinutes)) {
		http.Error(w, "challenge time has elapsed", 409)
		return
	}

	var body struct {
		QuestionID int    `json:"question_id"`
		Code       string `json:"code"`
		Language   string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.QuestionID == 0 {
		http.Error(w, "question_id required", 400)
		return
	}
	if body.Code == "" {
		http.Error(w, "code required", 400)
		return
	}
	if body.Language == "" {
		body.Language = "go"
	}

	// Verify this question belongs to this challenge
	var isAssigned bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM challenge_questions WHERE challenge_id=$1 AND question_id=$2)`,
		id, body.QuestionID).Scan(&isAssigned)
	if !isAssigned {
		http.Error(w, "question not part of this challenge", 403)
		return
	}

	// Run tests server-side
	var testCasesJSON, testFile string
	DB.QueryRow(`SELECT COALESCE(test_cases,'[]'), COALESCE(test_file,'') FROM questions WHERE id=$1`,
		body.QuestionID).Scan(&testCasesJSON, &testFile)

	var runResult RunResult
	if body.Language == "go" && strings.TrimSpace(testFile) != "" {
		runResult = RunTest(body.Code, testFile)
	} else {
		var testCases []TestCase
		if jsonErr := json.Unmarshal([]byte(testCasesJSON), &testCases); jsonErr == nil && len(testCases) > 0 {
			runResult = RunAgainstTestCases(body.Language, body.Code, testCases)
		} else {
			out, runErr := RunCode(body.Language, body.Code, "")
			runResult = RunResult{Total: 0, Passed: 0, AllPassed: false}
			if runErr != "" {
				runResult.Results = []TestResult{{Index: 1, Error: runErr, Passed: false}}
			} else {
				runResult.Results = []TestResult{{Index: 1, Got: out, Passed: false,
					Error: "no test cases defined for this question — output shown above"}}
			}
		}
	}

	passed := runResult.AllPassed

	// Record in challenge_submissions (never general submissions)
	DB.Exec(`
		INSERT INTO challenge_submissions (challenge_id, user_id, question_id, passed)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (challenge_id, user_id, question_id)
		DO UPDATE SET passed = challenge_submissions.passed OR EXCLUDED.passed`,
		id, u.ID, body.QuestionID, passed)

	// Check if all questions solved
	var totalQ, solvedQ int
	DB.QueryRow(`SELECT COUNT(*) FROM challenge_questions WHERE challenge_id=$1`, id).Scan(&totalQ)
	DB.QueryRow(`SELECT COUNT(*) FROM challenge_submissions WHERE challenge_id=$1 AND user_id=$2 AND passed=TRUE`,
		id, u.ID).Scan(&solvedQ)

	allDone := totalQ > 0 && solvedQ >= totalQ

	if allDone {
		if isChallenger {
			DB.Exec(`UPDATE challenges SET challenger_finished_at=NOW() WHERE id=$1 AND challenger_finished_at IS NULL`, id)
		} else {
			DB.Exec(`UPDATE challenges SET opponent_finished_at=NOW() WHERE id=$1 AND opponent_finished_at IS NULL`, id)
		}
		// Try to complete the whole challenge if both finished
		TryCompleteChallenge(id, c.ChallengerID, c.OpponentID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ArenaSubmitResponse{
		RunResult:    runResult,
		Passed:       passed,
		Finished:     allDone,
		Disqualified: false,
	})
}

// GET /api/challenges/{id}/result
func HandleChallengeResult(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/challenges/", "/result")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	c, err := GetChallenge(id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if c.ChallengerID != u.ID && c.OpponentID != u.ID && !u.IsAdmin {
		http.Error(w, "forbidden", 403)
		return
	}

	result, err := GetChallengeResult(id)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GET /api/admin/challenges
func HandleAdminListChallenges(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "admin only", 403)
		return
	}

	rows, err := DB.Query(`
		SELECT c.id, c.challenger_id, c.opponent_id, c.scheduled_at, c.status,
		       COALESCE(c.winner_id,0), c.challenger_score, c.opponent_score,
		       COALESCE(c.duration_minutes,60),
		       c.challenger_entered_at, c.opponent_entered_at,
		       c.challenger_finished_at, c.opponent_finished_at,
		       c.created_at,
		       uc.name, uo.name
		FROM challenges c
		JOIN users uc ON uc.id = c.challenger_id
		JOIN users uo ON uo.id = c.opponent_id
		ORDER BY c.created_at DESC`)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	defer rows.Close()

	var out []ChallengeDetail
	for rows.Next() {
		var d ChallengeDetail
		rows.Scan(
			&d.ID, &d.ChallengerID, &d.OpponentID, &d.ScheduledAt, &d.Status,
			&d.WinnerID, &d.ChallengerScore, &d.OpponentScore,
			&d.DurationMinutes,
			&d.ChallengerEnteredAt, &d.OpponentEnteredAt,
			&d.ChallengerFinishedAt, &d.OpponentFinishedAt,
			&d.CreatedAt,
			&d.ChallengerName, &d.OpponentName,
		)
		out = append(out, d)
	}
	if out == nil {
		out = []ChallengeDetail{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// POST /api/challenges/{id}/violation
func HandleChallengeViolation(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/challenges/", "/violation")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	c, err := GetChallenge(id)
	if err != nil {
		http.Error(w, "challenge not found", 404)
		return
	}
	if c.Status != "active" {
		http.Error(w, "challenge is not active", 409)
		return
	}
	if c.ChallengerID != u.ID && c.OpponentID != u.ID {
		http.Error(w, "you are not a participant", 403)
		return
	}

	var body struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}

	isChallenger := c.ChallengerID == u.ID

	if body.Type == "logout" {
		if isChallenger {
			DB.Exec(`UPDATE challenges SET challenger_disqualified=TRUE, challenger_finished_at=NOW() WHERE id=$1`, id)
		} else {
			DB.Exec(`UPDATE challenges SET opponent_disqualified=TRUE, opponent_finished_at=NOW() WHERE id=$1`, id)
		}
		TryCompleteChallenge(id, c.ChallengerID, c.OpponentID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"disqualified": true, "violation_count": 2})
		return
	}

	var newCount int
	if isChallenger {
		DB.QueryRow(`
			UPDATE challenges SET challenger_violations = challenger_violations + 1
			WHERE id=$1 RETURNING challenger_violations`, id).Scan(&newCount)
	} else {
		DB.QueryRow(`
			UPDATE challenges SET opponent_violations = opponent_violations + 1
			WHERE id=$1 RETURNING opponent_violations`, id).Scan(&newCount)
	}

	disqualified := newCount >= 2
	if disqualified {
		if isChallenger {
			DB.Exec(`UPDATE challenges SET challenger_disqualified=TRUE, challenger_finished_at=NOW() WHERE id=$1`, id)
		} else {
			DB.Exec(`UPDATE challenges SET opponent_disqualified=TRUE, opponent_finished_at=NOW() WHERE id=$1`, id)
		}
		TryCompleteChallenge(id, c.ChallengerID, c.OpponentID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"violation_count": newCount,
		"disqualified":    disqualified,
	})
}

// ── Shared helpers ────────────────────────────────────────────────────────────

func parseIDFromPath(path, prefix, suffix string) int {
	s := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		s = strings.TrimSuffix(s, suffix)
	}
	id, _ := strconv.Atoi(s)
	return id
}