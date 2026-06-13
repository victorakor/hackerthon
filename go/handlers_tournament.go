package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// POST /api/tournaments  (admin only)
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

// GET /api/tournaments
func HandleListTournaments(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	statusFilter := r.URL.Query().Get("status")

	query := `
		SELECT t.id, t.title, t.description, t.created_by, t.scheduled_at,
		       t.max_participants, t.status, COALESCE(t.duration_minutes,60), t.created_at,
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
			&d.MaxParticipants, &d.Status, &d.DurationMinutes, &d.CreatedAt,
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

	if overlap, err := HasContestOverlap(u.ID, t.ScheduledAt); err != nil || overlap {
		http.Error(w, "you already have a contest scheduled at that time", 409)
		return
	}

	_, err = DB.Exec(`
		INSERT INTO tournament_participants (tournament_id, user_id) VALUES ($1,$2)
		ON CONFLICT DO NOTHING`, id, u.ID)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

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

// POST /api/admin/tournaments/{id}/questions
// Body: { "question_ids": [2,5,8], "duration_minutes": 90 }
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

	DB.Exec(`UPDATE tournaments SET duration_minutes=$1 WHERE id=$2`, body.DurationMinutes, id)
	DB.Exec(`DELETE FROM tournament_questions WHERE tournament_id=$1`, id)
	for i, qid := range body.QuestionIDs {
		DB.Exec(`INSERT INTO tournament_questions (tournament_id, question_id, sort_order) VALUES ($1,$2,$3)
			     ON CONFLICT DO NOTHING`, id, qid, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /api/tournaments/{id}/enter
func HandleTournamentEnter(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/tournaments/", "/enter")
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
		http.Error(w, "tournament is not active yet", 409)
		return
	}

	// Verify participant
	var joined bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2)`,
		id, u.ID).Scan(&joined)
	if !joined {
		http.Error(w, "you are not a participant", 403)
		return
	}

	var disqualified bool
	DB.QueryRow(`SELECT COALESCE(disqualified,FALSE) FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2`,
		id, u.ID).Scan(&disqualified)
	if disqualified {
		http.Error(w, "you have been disqualified from this tournament", 403)
		return
	}

	var finished bool
	DB.QueryRow(`SELECT finished_at IS NOT NULL FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2`,
		id, u.ID).Scan(&finished)
	if finished {
		http.Error(w, "you have already completed this tournament", 409)
		return
	}

	endsAt := EndsAt(t.ScheduledAt, t.DurationMinutes)
	if time.Now().UTC().After(endsAt) {
		http.Error(w, "tournament time has elapsed", 409)
		return
	}

	// Set entered_at (idempotent)
	DB.Exec(`UPDATE tournament_participants SET entered_at=NOW() WHERE tournament_id=$1 AND user_id=$2 AND entered_at IS NULL`,
		id, u.ID)

	qs, err := GetTournamentQuestionObjects(id)
	if err != nil || len(qs) == 0 {
		http.Error(w, "no questions assigned to this tournament yet", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ArenaEnterResponse{
		EndsAt:          endsAt.UTC().Format(time.RFC3339),
		DurationMinutes: t.DurationMinutes,
		Questions:       qs,
	})
}

// POST /api/tournaments/{id}/arena-submit
// Body: { "question_id": 4, "code": "...", "language": "go" }
func HandleTournamentArenaSubmit(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/tournaments/", "/arena-submit")
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

	var joined bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2)`,
		id, u.ID).Scan(&joined)
	if !joined {
		http.Error(w, "you are not a participant", 403)
		return
	}

	var disqualified bool
	DB.QueryRow(`SELECT COALESCE(disqualified,FALSE) FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2`,
		id, u.ID).Scan(&disqualified)
	if disqualified {
		http.Error(w, "you have been disqualified", 403)
		return
	}

	var finished bool
	DB.QueryRow(`SELECT finished_at IS NOT NULL FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2`,
		id, u.ID).Scan(&finished)
	if finished {
		http.Error(w, "you have already completed this tournament", 409)
		return
	}

	if time.Now().UTC().After(EndsAt(t.ScheduledAt, t.DurationMinutes)) {
		http.Error(w, "tournament time has elapsed", 409)
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

	var isAssigned bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM tournament_questions WHERE tournament_id=$1 AND question_id=$2)`,
		id, body.QuestionID).Scan(&isAssigned)
	if !isAssigned {
		http.Error(w, "question not part of this tournament", 403)
		return
	}

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

	DB.Exec(`
		INSERT INTO tournament_submissions (tournament_id, user_id, question_id, passed)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (tournament_id, user_id, question_id)
		DO UPDATE SET passed = tournament_submissions.passed OR EXCLUDED.passed`,
		id, u.ID, body.QuestionID, passed)

	var totalQ, solvedQ int
	DB.QueryRow(`SELECT COUNT(*) FROM tournament_questions WHERE tournament_id=$1`, id).Scan(&totalQ)
	DB.QueryRow(`SELECT COUNT(*) FROM tournament_submissions WHERE tournament_id=$1 AND user_id=$2 AND passed=TRUE`,
		id, u.ID).Scan(&solvedQ)

	allDone := totalQ > 0 && solvedQ >= totalQ

	if allDone {
		DB.Exec(`UPDATE tournament_participants SET finished_at=NOW() WHERE tournament_id=$1 AND user_id=$2 AND finished_at IS NULL`,
			id, u.ID)
		TryCompleteTournament(id)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ArenaSubmitResponse{
		RunResult:    runResult,
		Passed:       passed,
		Finished:     allDone,
		Disqualified: false,
	})
}

// GET /api/tournaments/{id}/leaderboard
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

// DELETE /api/admin/tournaments/{id}
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

// POST /api/tournaments/{id}/violation
func HandleTournamentViolation(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/tournaments/", "/violation")
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

	var joined bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM tournament_participants WHERE tournament_id=$1 AND user_id=$2)`,
		id, u.ID).Scan(&joined)
	if !joined {
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

	if body.Type == "logout" {
		DB.Exec(`UPDATE tournament_participants
			SET disqualified=TRUE, disqualified_at=NOW(), finished_at=NOW()
			WHERE tournament_id=$1 AND user_id=$2`, id, u.ID)
		TryCompleteTournament(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"disqualified": true, "violation_count": 2})
		return
	}

	var newCount int
	DB.QueryRow(`
		UPDATE tournament_participants
		SET violation_count = violation_count + 1
		WHERE tournament_id=$1 AND user_id=$2
		RETURNING violation_count`, id, u.ID).Scan(&newCount)

	disqualified := newCount >= 2
	if disqualified {
		DB.Exec(`UPDATE tournament_participants
			SET disqualified=TRUE, disqualified_at=NOW(), finished_at=NOW()
			WHERE tournament_id=$1 AND user_id=$2`, id, u.ID)
		TryCompleteTournament(id)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"violation_count": newCount,
		"disqualified":    disqualified,
	})
}
