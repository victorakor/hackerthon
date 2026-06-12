package app

import (
	"encoding/json"
	"net/http"
	"time"
)

// POST /api/tournaments  (admin only)
// Body: { "title": "...", "description": "...", "scheduled_at": "RFC3339", "max_participants": 16 }
func HandleCreateTournament(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "admin only", 403)
		return
	}

	var body struct {
		Title           string `json:"title"`
		Description     string `json:"description"`
		ScheduledAt     string `json:"scheduled_at"`
		MaxParticipants int    `json:"max_participants"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if body.Title == "" {
		http.Error(w, "title required", 400)
		return
	}
	scheduledAt, err := time.Parse(time.RFC3339, body.ScheduledAt)
	if err != nil {
		http.Error(w, "scheduled_at must be RFC3339", 400)
		return
	}
	if body.MaxParticipants < 2 {
		body.MaxParticipants = 2
	}

	var id int
	err = DB.QueryRow(`
		INSERT INTO tournaments (title, description, created_by, scheduled_at, max_participants)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		body.Title, body.Description, u.ID, scheduledAt.UTC(), body.MaxParticipants,
	).Scan(&id)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "message": "Tournament created"})
}

// GET /api/tournaments  — list all tournaments (public)
// Optional query param: ?status=upcoming|active|completed
func HandleListTournaments(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	statusFilter := r.URL.Query().Get("status")

	query := `
		SELECT t.id, t.title, t.description, t.created_by, t.scheduled_at,
		       t.max_participants, t.status, t.created_at,
		       COUNT(tp.user_id) AS participant_count,
		       BOOL_OR(tp.user_id = $1) AS is_joined
		FROM tournaments t
		LEFT JOIN tournament_participants tp ON tp.tournament_id = t.id
	`
	args := []interface{}{u.ID}

	if statusFilter != "" {
		query += ` WHERE t.status = $2`
		args = append(args, statusFilter)
	}
	query += ` GROUP BY t.id ORDER BY t.scheduled_at DESC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	defer rows.Close()

	var out []TournamentDetail
	for rows.Next() {
		var d TournamentDetail
		rows.Scan(
			&d.ID, &d.Title, &d.Description, &d.CreatedBy, &d.ScheduledAt,
			&d.MaxParticipants, &d.Status, &d.CreatedAt,
			&d.ParticipantCount, &d.IsJoined,
		)
		out = append(out, d)
	}
	if out == nil {
		out = []TournamentDetail{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// GET /api/tournaments/{id}
func HandleGetTournament(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/tournaments/", "")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	t, err := GetTournament(id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}

	count, _ := GetTournamentParticipantCount(id)

	var isJoined bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2)`,
		id, u.ID).Scan(&isJoined)

	detail := TournamentDetail{
		Tournament:       *t,
		ParticipantCount: count,
		IsJoined:         isJoined,
	}

	// Attach questions when active or completed
	if t.Status == "active" || t.Status == "completed" {
		qids, _ := GetTournamentQuestions(id)
		detail.QuestionIDs = qids
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// POST /api/tournaments/{id}/join
func HandleJoinTournament(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/tournaments/", "/join")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	t, err := GetTournament(id)
	if err != nil {
		http.Error(w, "tournament not found", 404)
		return
	}
	if t.Status != "upcoming" {
		http.Error(w, "tournament is no longer open for registration", 409)
		return
	}

	count, _ := GetTournamentParticipantCount(id)
	if count >= t.MaxParticipants {
		http.Error(w, "tournament is full", 409)
		return
	}

	_, err = DB.Exec(`
		INSERT INTO tournament_participants (tournament_id, user_id) VALUES ($1,$2)
		ON CONFLICT DO NOTHING`, id, u.ID)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

	// Check if filling up triggered an immediate start
	TryActivateTournamentIfFull(id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "joined"})
}

// DELETE /api/tournaments/{id}/leave
func HandleLeaveTournament(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/tournaments/", "/leave")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	t, err := GetTournament(id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if t.Status != "upcoming" {
		http.Error(w, "cannot leave an active or completed tournament", 409)
		return
	}

	DB.Exec(`DELETE FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2`, id, u.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "left"})
}

// POST /api/admin/tournaments/{id}/questions  (admin only)
// Body: { "question_ids": [2, 5, 8] }
func HandleAssignTournamentQuestions(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "admin only", 403)
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/admin/tournaments/", "/questions")
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

	DB.Exec(`DELETE FROM tournament_questions WHERE tournament_id=$1`, id)
	for i, qid := range body.QuestionIDs {
		DB.Exec(`INSERT INTO tournament_questions (tournament_id, question_id, sort_order) VALUES ($1,$2,$3)
			     ON CONFLICT DO NOTHING`, id, qid, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /api/tournaments/{id}/submit
// Body: { "question_id": 4, "passed": true }
func HandleTournamentSubmit(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/tournaments/", "/submit")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	t, err := GetTournament(id)
	if err != nil {
		http.Error(w, "tournament not found", 404)
		return
	}
	if t.Status != "active" {
		http.Error(w, "tournament is not active", 409)
		return
	}

	// Verify user is a participant
	var joined bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2)`,
		id, u.ID).Scan(&joined)
	if !joined {
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

	_, err = DB.Exec(`
		INSERT INTO tournament_submissions (tournament_id, user_id, question_id, passed)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (tournament_id, user_id, question_id)
		DO UPDATE SET passed = tournament_submissions.passed OR EXCLUDED.passed`,
		id, u.ID, body.QuestionID, body.Passed)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

// GET /api/tournaments/{id}/leaderboard  — public after completion, participants-only while active
func HandleTournamentLeaderboard(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/tournaments/", "/leaderboard")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	t, err := GetTournament(id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if t.Status == "upcoming" {
		http.Error(w, "tournament has not started yet", 409)
		return
	}

	ranks, err := GetTournamentLeaderboard(id)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	if ranks == nil {
		ranks = []TournamentRank{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tournament_id": id,
		"title":         t.Title,
		"status":        t.Status,
		"ranks":         ranks,
	})
}

// DELETE /api/admin/tournaments/{id}  (admin only)
func HandleDeleteTournament(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil || !u.IsAdmin {
		http.Error(w, "admin only", 403)
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/admin/tournaments/", "")
	if id == 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	DB.Exec(`DELETE FROM tournaments WHERE id=$1`, id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}
