package app

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

func StartContestEngine() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			tickChallenges()
			tickTournaments()
			tickRaids()
		}
	}()
	log.Println("✓ Contest engine started")
}

// ── Challenges ────────────────────────────────────────────────────────────────

func tickChallenges() {
	now := time.Now().UTC()

	// 1. accepted challenges whose scheduled_at has arrived → activate
	rows, err := DB.Query(`
		SELECT id, challenger_id, opponent_id
		FROM challenges
		WHERE status = 'accepted' AND scheduled_at <= $1`, now)
	if err != nil {
		log.Printf("contest engine: query accepted challenges: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, challengerID, opponentID int
		rows.Scan(&id, &challengerID, &opponentID)
		activateChallenge(id, challengerID, opponentID)
	}

	// 2. active challenges whose time has elapsed OR both participants finished → complete
	rows2, err := DB.Query(`
		SELECT id, challenger_id, opponent_id
		FROM challenges
		WHERE status = 'active'
		AND (
			scheduled_at + (COALESCE(duration_minutes,60) * interval '1 minute') <= $1
			OR (challenger_finished_at IS NOT NULL AND opponent_finished_at IS NOT NULL)
		)`, now)
	if err != nil {
		log.Printf("contest engine: query active challenges: %v", err)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var id, challengerID, opponentID int
		rows2.Scan(&id, &challengerID, &opponentID)
		completeChallenge(id, challengerID, opponentID)
	}
}

func activateChallenge(id, challengerID, opponentID int) {
	// Fetch duration for the notification payload
	var durationMinutes int
	DB.QueryRow(`SELECT COALESCE(duration_minutes,60) FROM challenges WHERE id=$1`, id).Scan(&durationMinutes)

	_, err := DB.Exec(`UPDATE challenges SET status='active' WHERE id=$1`, id)
	if err != nil {
		log.Printf("contest engine: activate challenge %d: %v", id, err)
		return
	}
	log.Printf("contest engine: challenge %d now active", id)

	payload, _ := json.Marshal(map[string]interface{}{
		"challenge_id": id,
		"duration_min": durationMinutes,
	})
	p := string(payload)
	CreateNotification(challengerID, "contest_start", p)
	CreateNotification(opponentID, "contest_start", p)
}

func completeChallenge(id, challengerID, opponentID int) {
	// Ensure both participants have finished_at set (frees cooldown for timeout case)
	DB.Exec(`UPDATE challenges SET challenger_finished_at=NOW() WHERE id=$1 AND challenger_finished_at IS NULL`, id)
	DB.Exec(`UPDATE challenges SET opponent_finished_at=NOW() WHERE id=$1 AND opponent_finished_at IS NULL`, id)

	cScore, _ := CountChallengeScore(id, challengerID)
	oScore, _ := CountChallengeScore(id, opponentID)

	var winnerID int
	switch {
	case cScore > oScore:
		winnerID = challengerID
	case oScore > cScore:
		winnerID = opponentID
	default:
		winnerID = 0
	}

	_, err := DB.Exec(`
		UPDATE challenges
		SET status='completed', winner_id=NULLIF($1,0),
		    challenger_score=$2, opponent_score=$3
		WHERE id=$4`,
		winnerID, cScore, oScore, id)
	if err != nil {
		log.Printf("contest engine: complete challenge %d: %v", id, err)
		return
	}
	log.Printf("contest engine: challenge %d completed (c:%d o:%d winner:%d)",
		id, cScore, oScore, winnerID)

	payload, _ := json.Marshal(map[string]interface{}{
		"challenge_id":     id,
		"challenger_score": cScore,
		"opponent_score":   oScore,
		"winner_id":        winnerID,
	})
	p := string(payload)
	CreateNotification(challengerID, "contest_end", p)
	CreateNotification(opponentID, "contest_end", p)
}

// TryCompleteChallenge checks if both participants are done and completes early.
func TryCompleteChallenge(challengeID, challengerID, opponentID int) {
	var bothDone bool
	DB.QueryRow(`
		SELECT challenger_finished_at IS NOT NULL AND opponent_finished_at IS NOT NULL
		FROM challenges WHERE id=$1`, challengeID).Scan(&bothDone)
	if bothDone {
		completeChallenge(challengeID, challengerID, opponentID)
	}
}

// ── Tournaments ───────────────────────────────────────────────────────────────

func tickTournaments() {
	now := time.Now().UTC()

	// 1. upcoming tournaments whose scheduled_at has arrived → activate
	rows, err := DB.Query(`
		SELECT id FROM tournaments
		WHERE status = 'upcoming' AND scheduled_at <= $1`, now)
	if err != nil {
		log.Printf("contest engine: query upcoming tournaments: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		rows.Scan(&id)
		activateTournament(id)
	}

	// 2. active tournaments past duration OR all participants finished → complete
	rows2, err := DB.Query(`
		SELECT id FROM tournaments
		WHERE status = 'active'
		AND (
			scheduled_at + (COALESCE(duration_minutes,60) * interval '1 minute') <= $1
			OR NOT EXISTS (
				SELECT 1 FROM tournament_participants
				WHERE tournament_id = tournaments.id AND finished_at IS NULL
			)
		)`, now)
	if err != nil {
		log.Printf("contest engine: query active tournaments: %v", err)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var id int
		rows2.Scan(&id)
		completeTournament(id)
	}
}

func activateTournament(id int) {
	var durationMinutes int
	DB.QueryRow(`SELECT COALESCE(duration_minutes,60) FROM tournaments WHERE id=$1`, id).Scan(&durationMinutes)

	_, err := DB.Exec(`UPDATE tournaments SET status='active' WHERE id=$1`, id)
	if err != nil {
		log.Printf("contest engine: activate tournament %d: %v", id, err)
		return
	}
	log.Printf("contest engine: tournament %d now active", id)
	notifyTournamentParticipants(id, "tournament_start", map[string]interface{}{
		"tournament_id": id,
		"duration_min":  durationMinutes,
	})
}

func completeTournament(id int) {
	// Ensure all participants have finished_at (frees cooldown)
	DB.Exec(`UPDATE tournament_participants SET finished_at=NOW() WHERE tournament_id=$1 AND finished_at IS NULL`, id)

	_, err := DB.Exec(`UPDATE tournaments SET status='completed' WHERE id=$1`, id)
	if err != nil {
		log.Printf("contest engine: complete tournament %d: %v", id, err)
		return
	}
	log.Printf("contest engine: tournament %d completed", id)
	notifyTournamentParticipants(id, "tournament_end", map[string]interface{}{
		"tournament_id": id,
	})
}

// TryCompleteTournament checks if all participants are done and completes early.
func TryCompleteTournament(tournamentID int) {
	var anyActive bool
	DB.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM tournament_participants
		             WHERE tournament_id=$1 AND finished_at IS NULL)`,
		tournamentID).Scan(&anyActive)
	if !anyActive {
		completeTournament(tournamentID)
	}
}

func notifyTournamentParticipants(tournamentID int, kind string, data map[string]interface{}) {
	rows, err := DB.Query(
		`SELECT user_id FROM tournament_participants WHERE tournament_id=$1`, tournamentID)
	if err != nil {
		log.Printf("contest engine: fetch participants for tournament %d: %v", tournamentID, err)
		return
	}
	defer rows.Close()
	payload, _ := json.Marshal(data)
	p := string(payload)
	for rows.Next() {
		var uid int
		rows.Scan(&uid)
		if _, err := CreateNotification(uid, kind, p); err != nil {
			log.Printf("contest engine: notify user %d: %v", uid, err)
		}
	}
}

func TryActivateTournamentIfFull(tournamentID int) {
	t, err := GetTournament(tournamentID)
	if err != nil || t.Status != "upcoming" {
		return
	}
	count, err := GetTournamentParticipantCount(tournamentID)
	if err != nil {
		return
	}
	if count >= t.MaxParticipants && time.Now().UTC().After(t.ScheduledAt) {
		activateTournament(tournamentID)
	} else if count >= t.MaxParticipants {
		log.Printf("contest engine: tournament %d is full (%d/%d), waiting for scheduled time %s",
			tournamentID, count, t.MaxParticipants, t.ScheduledAt.Format(time.RFC3339))
	}
}

// EndsAt returns the time a contest ends given its scheduled start and duration.
func EndsAt(scheduledAt time.Time, durationMinutes int) time.Time {
	return scheduledAt.Add(time.Duration(durationMinutes) * time.Minute)
}

// SecondsRemaining returns seconds left in an active contest, or 0 if over.
func SecondsRemaining(scheduledAt time.Time, durationMinutes int) int {
	remaining := time.Until(EndsAt(scheduledAt, durationMinutes)).Seconds()
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}

// FormatContestPayload builds the JSON payload sent to the frontend on contest_start.
func FormatContestPayload(contestType string, id int, scheduledAt time.Time, durationMinutes int, questionIDs []int) string {
	b, _ := json.Marshal(map[string]interface{}{
		"type":         contestType,
		"id":           id,
		"ends_at":      EndsAt(scheduledAt, durationMinutes).UTC().Format(time.RFC3339),
		"question_ids": questionIDs,
		"duration_min": durationMinutes,
	})
	return fmt.Sprintf("%s", b)
}

// ── Raids ─────────────────────────────────────────────────────────────────────

func tickRaids() {
	now := time.Now().UTC()

	// 1. upcoming raids whose scheduled_at has arrived → activate
	rows, err := DB.Query(`
		SELECT id, initiating_clan_id FROM raids
		WHERE status = 'upcoming' AND scheduled_at <= $1`, now)
	if err != nil {
		log.Printf("raid engine: query upcoming raids: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, initiatingClanID int
		rows.Scan(&id, &initiatingClanID)
		activateRaid(id)
	}

	// 2. active raids past their duration → complete
	rows2, err := DB.Query(`
		SELECT id FROM raids
		WHERE status = 'active'
		AND scheduled_at + (duration_minutes * interval '1 minute') <= $1`, now)
	if err != nil {
		log.Printf("raid engine: query active raids: %v", err)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var id int
		rows2.Scan(&id)
		completeRaid(id)
	}
}

func activateRaid(raidID int) {
	// Count total participants across all clans in this raid
	var totalParticipants int
	DB.QueryRow(`
		SELECT COUNT(*)
		FROM clan_members cm
		JOIN raid_clans rc ON rc.clan_id = cm.clan_id
		WHERE rc.raid_id=$1`, raidID).Scan(&totalParticipants)

	// Calculate question count: CEIL(total/5), clamped 3–20
	questionCount := (totalParticipants + 4) / 5
	if questionCount < 3 {
		questionCount = 3
	}
	if questionCount > 20 {
		questionCount = 20
	}

	// Duration: 12 minutes per question
	durationMinutes := questionCount * 12

	// Randomly select questions from visible pool
	// Weight toward medium/hard by ordering: hard first, then medium, then easy,
	// with random shuffle within each tier via RANDOM()
	rows, err := DB.Query(`
		SELECT id FROM questions
		WHERE visible = TRUE
		ORDER BY
			CASE difficulty
				WHEN 'hard'   THEN 0
				WHEN 'medium' THEN 1
				ELSE               2
			END,
			RANDOM()
		LIMIT $1`, questionCount)
	if err != nil {
		log.Printf("raid engine: select questions for raid %d: %v", raidID, err)
		return
	}
	defer rows.Close()

	var questionIDs []int
	for rows.Next() {
		var qid int
		rows.Scan(&qid)
		questionIDs = append(questionIDs, qid)
	}

	if len(questionIDs) == 0 {
		log.Printf("raid engine: no visible questions available for raid %d", raidID)
		return
	}

	// Insert into raid_questions
	for i, qid := range questionIDs {
		DB.Exec(`
			INSERT INTO raid_questions (raid_id, question_id, sort_order)
			VALUES ($1,$2,$3)
			ON CONFLICT DO NOTHING`, raidID, qid, i)
	}

	// Update raid status, question_count, duration_minutes
	_, err = DB.Exec(`
		UPDATE raids
		SET status='active', question_count=$1, duration_minutes=$2
		WHERE id=$3`,
		len(questionIDs), durationMinutes, raidID)
	if err != nil {
		log.Printf("raid engine: activate raid %d: %v", raidID, err)
		return
	}
	log.Printf("raid engine: raid %d now active (%d questions, %d min)",
		raidID, len(questionIDs), durationMinutes)

	// Notify all members of all clans in this raid
	notifyAllRaidMembers(raidID, "raid_start", map[string]interface{}{
		"raid_id":          raidID,
		"question_count":   len(questionIDs),
		"duration_minutes": durationMinutes,
		"message":          "A raid has started! Enter the arena now.",
	})
}

func completeRaid(raidID int) {
	// Rank clans by final score
	rows, err := DB.Query(`
		SELECT clan_id, score FROM raid_clans
		WHERE raid_id=$1
		ORDER BY score DESC, clan_id ASC`, raidID)
	if err != nil {
		log.Printf("raid engine: rank clans for raid %d: %v", raidID, err)
		return
	}
	defer rows.Close()

	var clans []raidClanRankEntry
	for rows.Next() {
		var cs raidClanRankEntry
		rows.Scan(&cs.clanID, &cs.score)
		clans = append(clans, cs)
	}

	// Assign ranks and apply Elo adjustments
	for i, cs := range clans {
		rank := i + 1
		DB.Exec(`UPDATE raid_clans SET rank=$1 WHERE raid_id=$2 AND clan_id=$3`,
			rank, raidID, cs.clanID)
	}

	// Apply Elo pairwise between all clans
	applyRaidElo(clans)

	// Mark completed
	_, err = DB.Exec(`UPDATE raids SET status='completed' WHERE id=$1`, raidID)
	if err != nil {
		log.Printf("raid engine: complete raid %d: %v", raidID, err)
		return
	}
	log.Printf("raid engine: raid %d completed", raidID)

	// Build result summary for notification
	resultData := map[string]interface{}{
		"raid_id": raidID,
		"message": "Raid complete! Check the results.",
	}
	if len(clans) > 0 {
		resultData["winner_clan_id"] = clans[0].clanID
	}
	notifyAllRaidMembers(raidID, "raid_end", resultData)
}

// raidClanRankEntry holds a clan's id and final score for ranking/Elo.
type raidClanRankEntry struct {
	clanID int
	score  int
}

// applyRaidElo applies pairwise Elo adjustments between all clans in a raid.
// clans must be sorted by score DESC (winner first).
func applyRaidElo(clans []raidClanRankEntry) {
	// Fetch current ratings
	ratings := map[int]float64{}
	for _, cs := range clans {
		var rating int
		DB.QueryRow(`SELECT rating FROM clans WHERE id=$1`, cs.clanID).Scan(&rating)
		ratings[cs.clanID] = float64(rating)
	}

	adjustments := map[int]float64{}
	for i := 0; i < len(clans); i++ {
		for j := i + 1; j < len(clans); j++ {
			w := clans[i] // higher score = winner
			l := clans[j] // lower score = loser

			rW := ratings[w.clanID]
			rL := ratings[l.clanID]

			expectedW := 1.0 / (1.0 + pow10((rL-rW)/400.0))
			expectedL := 1.0 - expectedW

			// K-factor 32
			adjustments[w.clanID] += 32.0 * (1.0 - expectedW)
			adjustments[l.clanID] += 32.0 * (0.0 - expectedL)
		}
	}

	for clanID, delta := range adjustments {
		newRating := int(ratings[clanID] + delta)
		if newRating < 0 {
			newRating = 0
		}
		DB.Exec(`UPDATE clans SET rating=$1 WHERE id=$2`, newRating, clanID)
	}
}

// pow10 computes 10^x using integer-friendly approximation
func pow10(x float64) float64 {
	// math.Pow(10, x) — we inline to avoid importing math in this file
	// Use the identity 10^x = e^(x * ln10)
	// Since we can't import math easily without adding to imports,
	// we use a simple iterative approach for the range we need.
	// For Elo, x is typically between -3 and 3.
	result := 1.0
	base := 10.0
	if x == 0 {
		return 1.0
	}
	negative := x < 0
	if negative {
		x = -x
	}
	// Integer part
	intPart := int(x)
	fracPart := x - float64(intPart)
	for i := 0; i < intPart; i++ {
		result *= base
	}
	// Fractional part via series: 10^f = e^(f*ln10)
	// ln(10) ≈ 2.302585
	ln10 := 2.302585092994046
	ef := fracPart * ln10
	// e^ef via Taylor series (good enough for |ef| < 3)
	term := 1.0
	eFrac := 1.0
	for i := 1; i <= 20; i++ {
		term *= ef / float64(i)
		eFrac += term
	}
	result *= eFrac
	if negative {
		return 1.0 / result
	}
	return result
}

func notifyAllRaidMembers(raidID int, kind string, data map[string]interface{}) {
	rows, err := DB.Query(`
		SELECT DISTINCT cm.user_id
		FROM raid_clans rc
		JOIN clan_members cm ON cm.clan_id = rc.clan_id
		WHERE rc.raid_id=$1`, raidID)
	if err != nil {
		log.Printf("raid engine: notify members for raid %d: %v", raidID, err)
		return
	}
	defer rows.Close()

	payload, _ := json.Marshal(data)
	p := string(payload)
	for rows.Next() {
		var uid int
		rows.Scan(&uid)
		if _, err := CreateNotification(uid, kind, p); err != nil {
			log.Printf("raid engine: notify user %d: %v", uid, err)
		}
	}
}
