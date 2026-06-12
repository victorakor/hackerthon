package app

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// StartContestEngine launches a background goroutine that ticks every 30 seconds
// and transitions challenges/tournaments between states automatically.
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

	// 2. active challenges whose scheduled_at + 1 hour has passed → complete
	rows2, err := DB.Query(`
		SELECT id, challenger_id, opponent_id
		FROM challenges
		WHERE status = 'active' AND scheduled_at + INTERVAL '1 hour' <= $1`, now)
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
	_, err := DB.Exec(`UPDATE challenges SET status='active' WHERE id=$1`, id)
	if err != nil {
		log.Printf("contest engine: activate challenge %d: %v", id, err)
		return
	}
	log.Printf("contest engine: challenge %d now active", id)

	payload, _ := json.Marshal(map[string]interface{}{
		"challenge_id": id,
		"duration_min": 60,
	})
	p := string(payload)
	CreateNotification(challengerID, "contest_start", p)
	CreateNotification(opponentID, "contest_start", p)
}

func completeChallenge(id, challengerID, opponentID int) {
	cScore, _ := CountChallengeScore(id, challengerID)
	oScore, _ := CountChallengeScore(id, opponentID)

	var winnerID int
	switch {
	case cScore > oScore:
		winnerID = challengerID
	case oScore > cScore:
		winnerID = opponentID
	default:
		winnerID = 0 // tie
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

	// 2. active tournaments past scheduled_at + 1 hour → complete
	rows2, err := DB.Query(`
		SELECT id FROM tournaments
		WHERE status = 'active' AND scheduled_at + INTERVAL '1 hour' <= $1`, now)
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
	_, err := DB.Exec(`UPDATE tournaments SET status='active' WHERE id=$1`, id)
	if err != nil {
		log.Printf("contest engine: activate tournament %d: %v", id, err)
		return
	}
	log.Printf("contest engine: tournament %d now active", id)
	notifyTournamentParticipants(id, "tournament_start", map[string]interface{}{
		"tournament_id": id,
		"duration_min":  60,
	})
}

func completeTournament(id int) {
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

// TryActivateTournamentIfFull is called by the join handler.
// If the tournament just hit max_participants, activate it immediately
// (only if scheduled_at has also already passed; otherwise the ticker handles it).
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
		// Full but not yet time — log it, ticker will start at scheduled_at
		log.Printf("contest engine: tournament %d is full (%d/%d), waiting for scheduled time %s",
			tournamentID, count, t.MaxParticipants, t.ScheduledAt.Format(time.RFC3339))
	}
}

// EndsAt returns the time a contest ends given its scheduled start.
func EndsAt(scheduledAt time.Time) time.Time {
	return scheduledAt.Add(time.Hour)
}

// SecondsRemaining returns seconds left in an active contest, or 0 if over.
func SecondsRemaining(scheduledAt time.Time) int {
	remaining := time.Until(EndsAt(scheduledAt)).Seconds()
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}

// FormatContestPayload builds the JSON payload sent to the frontend on contest_start.
func FormatContestPayload(contestType string, id int, scheduledAt time.Time, questionIDs []int) string {
	b, _ := json.Marshal(map[string]interface{}{
		"type":         contestType, // "challenge" | "tournament"
		"id":           id,
		"ends_at":      EndsAt(scheduledAt).UTC().Format(time.RFC3339),
		"question_ids": questionIDs,
		"duration_min": 60,
	})
	return fmt.Sprintf("%s", b)
}
