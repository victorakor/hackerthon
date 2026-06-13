package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// POST /api/raids
func HandleCreateRaid(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	// Must be clanhead or general
	var myClanID int
	var myRole string
	err := DB.QueryRow(`SELECT clan_id, role FROM clan_members WHERE user_id=$1`, u.ID).
		Scan(&myClanID, &myRole)
	if err != nil {
		http.Error(w, "you are not in a clan", 403)
		return
	}
	if myRole != "clanhead" && myRole != "general" {
		http.Error(w, "only clanhead or general can create a raid", 403)
		return
	}

	var body struct {
		ScheduledAt   string `json:"scheduled_at"`
		TargetClanIDs []int  `json:"target_clan_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if len(body.TargetClanIDs) == 0 {
		http.Error(w, "target_clan_ids required", 400)
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

	// Cannot include your own clan in target list
	for _, tid := range body.TargetClanIDs {
		if tid == myClanID {
			http.Error(w, "cannot raid your own clan", 400)
			return
		}
	}

	// No duplicate target clan IDs
	seen := map[int]bool{}
	for _, tid := range body.TargetClanIDs {
		if seen[tid] {
			http.Error(w, "duplicate clan in target list", 400)
			return
		}
		seen[tid] = true
	}

	// Check all target clans exist
	for _, tid := range body.TargetClanIDs {
		var exists bool
		DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clans WHERE id=$1)`, tid).Scan(&exists)
		if !exists {
			http.Error(w, "one or more target clans not found", 404)
			return
		}
	}

	// No overlapping upcoming/active raid for initiating clan
	var hasActiveRaid bool
	DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM raid_clans rc
			JOIN raids rd ON rd.id = rc.raid_id
			WHERE rc.clan_id=$1 AND rd.status IN ('upcoming','active')
		)`, myClanID).Scan(&hasActiveRaid)
	if hasActiveRaid {
		http.Error(w, "your clan already has an upcoming or active raid", 409)
		return
	}

	// Insert raid
	var raidID int
	err = DB.QueryRow(`
		INSERT INTO raids (initiating_clan_id, scheduled_at, created_by)
		VALUES ($1,$2,$3) RETURNING id`,
		myClanID, scheduledAt.UTC(), u.ID,
	).Scan(&raidID)
	if err != nil {
		http.Error(w, "failed to create raid", 500)
		return
	}

	// Add initiating clan to raid_clans
	DB.Exec(`INSERT INTO raid_clans (raid_id, clan_id) VALUES ($1,$2)`, raidID, myClanID)

	// Add all target clans
	for _, tid := range body.TargetClanIDs {
		DB.Exec(`INSERT INTO raid_clans (raid_id, clan_id) VALUES ($1,$2)`, raidID, tid)
	}

	// Notify initiating clan members
	notifyRaidClansMembers(raidID, myClanID, "raid_created", map[string]interface{}{
		"raid_id":      raidID,
		"scheduled_at": scheduledAt.UTC().Format(time.RFC3339),
		"message":      "Your clan has set a raid!",
	})

	// Notify target clan members
	for _, tid := range body.TargetClanIDs {
		notifyRaidClansMembers(raidID, tid, "raid_incoming", map[string]interface{}{
			"raid_id":      raidID,
			"scheduled_at": scheduledAt.UTC().Format(time.RFC3339),
			"message":      "Your clan has been challenged to a raid!",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      raidID,
		"message": "Raid scheduled",
	})
}

// GET /api/raids  — list raids relevant to user's clan (upcoming + active + recent completed)
func HandleListRaids(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	// Get user's clan (if any) to surface their raids first
	var myClanID int
	DB.QueryRow(`SELECT clan_id FROM clan_members WHERE user_id=$1`, u.ID).Scan(&myClanID)

	rows, err := DB.Query(`
		SELECT DISTINCT rd.id, rd.initiating_clan_id, rd.scheduled_at, rd.status,
		       rd.question_count, rd.duration_minutes, rd.created_by, rd.created_at,
		       c.name AS initiating_clan_name
		FROM raids rd
		JOIN clans c ON c.id = rd.initiating_clan_id
		JOIN raid_clans rc ON rc.raid_id = rd.id
		WHERE rd.status IN ('upcoming','active')
		   OR (rd.status = 'completed' AND rd.scheduled_at > NOW() - INTERVAL '7 days')
		ORDER BY rd.scheduled_at DESC
		LIMIT 50`)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	defer rows.Close()

	var out []RaidDetail
	for rows.Next() {
		var d RaidDetail
		rows.Scan(
			&d.ID, &d.InitiatingClanID, &d.ScheduledAt, &d.Status,
			&d.QuestionCount, &d.DurationMinutes, &d.CreatedBy, &d.CreatedAt,
			&d.InitiatingClanName,
		)
		d.Clans = getRaidClanScores(d.ID)
		if d.Status == "active" {
			d.EndsAt = EndsAt(d.ScheduledAt, d.DurationMinutes).UTC().Format(time.RFC3339)
		}
		out = append(out, d)
	}
	if out == nil {
		out = []RaidDetail{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// GET /api/raids/{id}
func HandleGetRaid(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/raids/", "")
	if id == 0 {
		http.Error(w, "invalid raid id", 400)
		return
	}

	d, err := getRaidDetail(id)
	if err != nil {
		http.Error(w, "raid not found", 404)
		return
	}
	_ = u
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

// POST /api/raids/{id}/enter
func HandleRaidEnter(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/raids/", "/enter")
	if id == 0 {
		http.Error(w, "invalid raid id", 400)
		return
	}

	// Raid must be active
	var status string
	var scheduledAt time.Time
	var durationMinutes int
	err := DB.QueryRow(`
		SELECT status, scheduled_at, duration_minutes FROM raids WHERE id=$1`, id).
		Scan(&status, &scheduledAt, &durationMinutes)
	if err != nil {
		http.Error(w, "raid not found", 404)
		return
	}
	if status != "active" {
		http.Error(w, "raid is not active yet", 409)
		return
	}
	if time.Now().UTC().After(EndsAt(scheduledAt, durationMinutes)) {
		http.Error(w, "raid has already ended", 409)
		return
	}

	// User must be in a participating clan
	var myClanID int
	DB.QueryRow(`SELECT clan_id FROM clan_members WHERE user_id=$1`, u.ID).Scan(&myClanID)
	if myClanID == 0 {
		http.Error(w, "you are not in a clan", 403)
		return
	}

	var inRaid bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM raid_clans WHERE raid_id=$1 AND clan_id=$2)`,
		id, myClanID).Scan(&inRaid)
	if !inRaid {
		http.Error(w, "your clan is not part of this raid", 403)
		return
	}

	// Block disqualified or finished users — they can only watch results
	var isDisqualified, isFinished bool
	DB.QueryRow(`SELECT disqualified, COALESCE(finished, FALSE) FROM raid_violations WHERE raid_id=$1 AND user_id=$2`,
		id, u.ID).Scan(&isDisqualified, &isFinished)
	if isDisqualified {
		http.Error(w, "you have been disqualified from this raid", 403)
		return
	}
	if isFinished {
		http.Error(w, "you have already finished this raid", 403)
		return
	}

	// Fetch questions
	questions, err := getRaidQuestions(id)
	if err != nil || len(questions) == 0 {
		http.Error(w, "raid questions not ready", 500)
		return
	}

	endsAt := EndsAt(scheduledAt, durationMinutes).UTC().Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RaidArenaResponse{
		EndsAt:          endsAt,
		DurationMinutes: durationMinutes,
		Questions:       questions,
		ClanScores:      getRaidClanScores(id),
	})
}

// POST /api/raids/{id}/arena-submit
func HandleRaidArenaSubmit(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/raids/", "/arena-submit")
	if id == 0 {
		http.Error(w, "invalid raid id", 400)
		return
	}

	// Raid must be active and within time window
	var status string
	var scheduledAt time.Time
	var durationMinutes int
	err := DB.QueryRow(`
		SELECT status, scheduled_at, duration_minutes FROM raids WHERE id=$1`, id).
		Scan(&status, &scheduledAt, &durationMinutes)
	if err != nil {
		http.Error(w, "raid not found", 404)
		return
	}
	if status != "active" {
		http.Error(w, "raid is not active", 409)
		return
	}
	if time.Now().UTC().After(EndsAt(scheduledAt, durationMinutes)) {
		http.Error(w, "raid time has elapsed", 409)
		return
	}

	// User must be in a participating clan
	var myClanID int
	DB.QueryRow(`SELECT clan_id FROM clan_members WHERE user_id=$1`, u.ID).Scan(&myClanID)
	if myClanID == 0 {
		http.Error(w, "you are not in a clan", 403)
		return
	}
	var inRaid bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM raid_clans WHERE raid_id=$1 AND clan_id=$2)`,
		id, myClanID).Scan(&inRaid)
	if !inRaid {
		http.Error(w, "your clan is not part of this raid", 403)
		return
	}

	// Block disqualified or finished users from submitting
	var isDisqualifiedSub, isFinishedSub bool
	DB.QueryRow(`SELECT disqualified, COALESCE(finished, FALSE) FROM raid_violations WHERE raid_id=$1 AND user_id=$2`,
		id, u.ID).Scan(&isDisqualifiedSub, &isFinishedSub)
	if isDisqualifiedSub {
		http.Error(w, "you have been disqualified from this raid", 403)
		return
	}
	if isFinishedSub {
		http.Error(w, "you have already finished this raid", 403)
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

	// Verify question belongs to this raid
	var isAssigned bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM raid_questions WHERE raid_id=$1 AND question_id=$2)`,
		id, body.QuestionID).Scan(&isAssigned)
	if !isAssigned {
		http.Error(w, "question not part of this raid", 403)
		return
	}

	// Run tests server-side (same pattern as challenge/tournament)
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
					Error: "no test cases defined for this question"}}
			}
		}
	}

	passed := runResult.AllPassed

	// Record submission — once per user per question, upgrade if now passing
	DB.Exec(`
		INSERT INTO raid_submissions (raid_id, clan_id, user_id, question_id, passed)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (raid_id, user_id, question_id)
		DO UPDATE SET passed = raid_submissions.passed OR EXCLUDED.passed`,
		id, myClanID, u.ID, body.QuestionID, passed)

	// Recalculate and update this clan's score
	var newScore int
	DB.QueryRow(`
		SELECT COUNT(*) FROM raid_submissions
		WHERE raid_id=$1 AND clan_id=$2 AND passed=TRUE`,
		id, myClanID).Scan(&newScore)
	DB.Exec(`UPDATE raid_clans SET score=$1 WHERE raid_id=$2 AND clan_id=$3`,
		newScore, id, myClanID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RaidSubmitResponse{
		RunResult:  runResult,
		Passed:     passed,
		ClanScores: getRaidClanScores(id),
	})
}

// GET /api/raids/{id}/leaderboard
func HandleRaidLeaderboard(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/raids/", "/leaderboard")
	if id == 0 {
		http.Error(w, "invalid raid id", 400)
		return
	}

	scores := getRaidClanScores(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scores)
}

// POST /api/raids/{id}/finish
// Called when a user solves all questions or explicitly finishes. Marks them as done
// so they cannot re-enter the arena — they are redirected to the results view.
func HandleRaidFinish(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/raids/", "/finish")
	if id == 0 {
		http.Error(w, "invalid raid id", 400)
		return
	}

	// Upsert a violations row marking this user as finished
	DB.Exec(`
		INSERT INTO raid_violations (raid_id, user_id, clan_id, count, finished)
		SELECT $1, $2, cm.clan_id, 0, TRUE
		FROM clan_members cm WHERE cm.user_id=$2
		ON CONFLICT (raid_id, user_id) DO UPDATE SET finished=TRUE`,
		id, u.ID)

	d, err := getRaidDetail(id)
	if err != nil {
		http.Error(w, "raid not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"finished":    true,
		"clan_scores": d.Clans,
	})
}

// POST /api/raids/{id}/violation
// Called by AntiCheat.js when a copy/paste or tab-switch event fires during a raid.
func HandleRaidViolation(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/raids/", "/violation")
	if id == 0 {
		http.Error(w, "invalid raid id", 400)
		return
	}

	// Raid must be active
	var status string
	var scheduledAt time.Time
	var durationMinutes int
	err := DB.QueryRow(`SELECT status, scheduled_at, duration_minutes FROM raids WHERE id=$1`, id).
		Scan(&status, &scheduledAt, &durationMinutes)
	if err != nil {
		http.Error(w, "raid not found", 404)
		return
	}
	if status != "active" {
		http.Error(w, "raid is not active", 409)
		return
	}
	if time.Now().UTC().After(EndsAt(scheduledAt, durationMinutes)) {
		http.Error(w, "raid has already ended", 409)
		return
	}

	// User must be in a participating clan
	var myClanID int
	DB.QueryRow(`SELECT clan_id FROM clan_members WHERE user_id=$1`, u.ID).Scan(&myClanID)
	if myClanID == 0 {
		http.Error(w, "you are not in a clan", 403)
		return
	}
	var inRaid bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM raid_clans WHERE raid_id=$1 AND clan_id=$2)`,
		id, myClanID).Scan(&inRaid)
	if !inRaid {
		http.Error(w, "your clan is not part of this raid", 403)
		return
	}

	var body struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}

	// Upsert violation row then increment
	DB.Exec(`
		INSERT INTO raid_violations (raid_id, user_id, clan_id, count)
		VALUES ($1,$2,$3,0)
		ON CONFLICT (raid_id, user_id) DO NOTHING`,
		id, u.ID, myClanID)

	// Instant disqualification on logout (tab close / navigate away)
	if body.Type == "logout" {
		DB.Exec(`
			UPDATE raid_violations SET disqualified=TRUE
			WHERE raid_id=$1 AND user_id=$2`, id, u.ID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"disqualified": true, "violation_count": 2})
		return
	}

	var newCount int
	DB.QueryRow(`
		UPDATE raid_violations SET count=count+1
		WHERE raid_id=$1 AND user_id=$2
		RETURNING count`, id, u.ID).Scan(&newCount)

	disqualified := newCount >= 2
	if disqualified {
		DB.Exec(`
			UPDATE raid_violations SET disqualified=TRUE
			WHERE raid_id=$1 AND user_id=$2`, id, u.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"violation_count": newCount,
		"disqualified":    disqualified,
	})
}

// ── Raid DB helpers ───────────────────────────────────────────────────────────

func getRaidDetail(raidID int) (*RaidDetail, error) {
	var d RaidDetail
	err := DB.QueryRow(`
		SELECT rd.id, rd.initiating_clan_id, rd.scheduled_at, rd.status,
		       rd.question_count, rd.duration_minutes, rd.created_by, rd.created_at,
		       c.name
		FROM raids rd
		JOIN clans c ON c.id = rd.initiating_clan_id
		WHERE rd.id=$1`, raidID).Scan(
		&d.ID, &d.InitiatingClanID, &d.ScheduledAt, &d.Status,
		&d.QuestionCount, &d.DurationMinutes, &d.CreatedBy, &d.CreatedAt,
		&d.InitiatingClanName,
	)
	if err != nil {
		return nil, err
	}
	d.Clans = getRaidClanScores(raidID)
	if d.Status == "active" {
		d.EndsAt = EndsAt(d.ScheduledAt, d.DurationMinutes).UTC().Format(time.RFC3339)
	}
	return &d, nil
}

func getRaidClanScores(raidID int) []RaidClanScore {
	rows, err := DB.Query(`
		SELECT rc.clan_id, c.name, rc.score, COALESCE(rc.rank,0)
		FROM raid_clans rc
		JOIN clans c ON c.id = rc.clan_id
		WHERE rc.raid_id=$1
		ORDER BY rc.score DESC, c.name ASC`, raidID)
	if err != nil {
		return []RaidClanScore{}
	}
	defer rows.Close()

	var out []RaidClanScore
	pos := 1
	for rows.Next() {
		var s RaidClanScore
		rows.Scan(&s.ClanID, &s.ClanName, &s.Score, &s.Rank)
		if s.Rank == 0 {
			s.Rank = pos
		}
		out = append(out, s)
		pos++
	}
	if out == nil {
		return []RaidClanScore{}
	}
	return out
}

func getRaidQuestions(raidID int) ([]Question, error) {
	rows, err := DB.Query(`
		SELECT q.id, q.title, q.description, q.difficulty, q.category,
		       q.hint_url, q.hint_text, q.visible,
		       COALESCE(q.test_cases,'[]'), COALESCE(q.test_file,'')
		FROM raid_questions rq
		JOIN questions q ON q.id = rq.question_id
		WHERE rq.raid_id=$1
		ORDER BY rq.sort_order`, raidID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var qs []Question
	for rows.Next() {
		var q Question
		rows.Scan(&q.ID, &q.Title, &q.Description, &q.Difficulty, &q.Category,
			&q.HintURL, &q.HintText, &q.Visible, &q.TestCases, &q.TestFile)
		qs = append(qs, q)
	}
	return qs, nil
}

// notifyRaidClansMembers sends a notification to all members of a specific clan in a raid
func notifyRaidClansMembers(raidID, clanID int, kind string, data map[string]interface{}) {
	rows, err := DB.Query(`SELECT user_id FROM clan_members WHERE clan_id=$1`, clanID)
	if err != nil {
		return
	}
	defer rows.Close()
	payload, _ := json.Marshal(data)
	p := string(payload)
	for rows.Next() {
		var uid int
		rows.Scan(&uid)
		CreateNotification(uid, kind, p)
	}
}
