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
// Body: { "opponent_id": 5, "scheduled_at": "2026-06-13T18:00:00Z" }
func HandleCreateChallenge(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	var body struct {
		OpponentID  int    `json:"opponent_id"`
		ScheduledAt string `json:"scheduled_at"` // RFC3339 UTC from frontend
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

	// Verify opponent exists
	var exists bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, body.OpponentID).Scan(&exists)
	if !exists {
		http.Error(w, "opponent not found", 404)
		return
	}

	// Check for an already-pending/accepted challenge between these two users
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

	// Notify the opponent
	payload, _ := json.Marshal(map[string]interface{}{
		"challenge_id":    id,
		"challenger_id":   u.ID,
		"challenger_name": u.Name,
		"scheduled_at":    scheduledAt.UTC().Format(time.RFC3339),
	})
	CreateNotification(body.OpponentID, "challenge_received", string(payload))

	// Notify all admins
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

	_, err = DB.Exec(`UPDATE challenges SET status='accepted' WHERE id=$1`, id)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

	// Notify challenger
	payload, _ := json.Marshal(map[string]interface{}{
		"challenge_id":  id,
		"opponent_name": u.Name,
		"scheduled_at":  c.ScheduledAt.UTC().Format(time.RFC3339),
	})
	CreateNotification(c.ChallengerID, "challenge_accepted", string(payload))

	// Notify admins to assign questions
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

// GET /api/challenges  — list challenges the current user is part of
func HandleListChallenges(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	rows, err := DB.Query(`
		SELECT c.id, c.challenger_id, c.opponent_id, c.scheduled_at, c.status,
		       COALESCE(c.winner_id, 0), c.challenger_score, c.opponent_score, c.created_at,
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
			&d.WinnerID, &d.ChallengerScore, &d.OpponentScore, &d.CreatedAt,
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

// GET /api/challenges/{id}  — detail + questions (if active/completed) + result
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

	// Only participants (or admins) can see a challenge
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

	// Attach questions only when active or completed
	if c.Status == "active" || c.Status == "completed" {
		qids, _ := GetChallengeQuestions(id)
		detail.QuestionIDs = qids
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// POST /api/admin/challenges/{id}/questions
// Body: { "question_ids": [1, 4, 7] }
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
		QuestionIDs []int `json:"question_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.QuestionIDs) == 0 {
		http.Error(w, "question_ids required", 400)
		return
	}

	// Replace existing assignments
	DB.Exec(`DELETE FROM challenge_questions WHERE challenge_id=$1`, id)
	for i, qid := range body.QuestionIDs {
		DB.Exec(`INSERT INTO challenge_questions (challenge_id, question_id, sort_order) VALUES ($1,$2,$3)
			     ON CONFLICT DO NOTHING`, id, qid, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /api/challenges/{id}/submit
// Records a passed question for the current user in this challenge.
// Body: { "question_id": 3, "passed": true }
func HandleChallengeSubmit(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/challenges/", "/submit")
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
		QuestionID int  `json:"question_id"`
		Passed     bool `json:"passed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.QuestionID == 0 {
		http.Error(w, "question_id required", 400)
		return
	}

	// Upsert — only record a pass, never downgrade a pass to a fail
	_, err = DB.Exec(`
		INSERT INTO challenge_submissions (challenge_id, user_id, question_id, passed)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (challenge_id, user_id, question_id)
		DO UPDATE SET passed = challenge_submissions.passed OR EXCLUDED.passed`,
		id, u.ID, body.QuestionID, body.Passed)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
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

// GET /api/admin/challenges  — all challenges (admin only)
func HandleAdminListChallenges(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "admin only", 403)
		return
	}

	rows, err := DB.Query(`
		SELECT c.id, c.challenger_id, c.opponent_id, c.scheduled_at, c.status,
		       COALESCE(c.winner_id,0), c.challenger_score, c.opponent_score, c.created_at,
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
			&d.WinnerID, &d.ChallengerScore, &d.OpponentScore, &d.CreatedAt,
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

// ── Shared helper ─────────────────────────────────────────────────────────────

// parseIDFromPath extracts the integer segment between prefix and suffix.
// e.g. "/api/challenges/42/accept", prefix="/api/challenges/", suffix="/accept" → 42
func parseIDFromPath(path, prefix, suffix string) int {
	s := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		s = strings.TrimSuffix(s, suffix)
	}
	id, _ := strconv.Atoi(s)
	return id
}
