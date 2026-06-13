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
