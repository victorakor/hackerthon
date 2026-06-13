package app

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type spaHandler struct {
	fs http.Handler
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := "static" + r.URL.Path
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		http.ServeFile(w, r, "static/index.html")
		return
	}
	h.fs.ServeHTTP(w, r)
}

func WithCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,PATCH,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next(w, r)
	}
}

func RegisterRoutes() {
	http.Handle("/", spaHandler{fs: http.FileServer(http.Dir("static"))})

	// Auth
	http.HandleFunc("/api/auth/register", WithCORS(HandleRegister))
	http.HandleFunc("/api/auth/login", WithCORS(HandleLogin))
	http.HandleFunc("/api/auth/logout", WithCORS(HandleLogout))
	http.HandleFunc("/api/auth/forgot-password", WithCORS(HandleForgotPassword))
	http.HandleFunc("/api/auth/reset-password", WithCORS(HandleResetPassword))
	http.HandleFunc("/api/auth/validate-reset-token", WithCORS(HandleValidateResetToken))
	http.HandleFunc("/api/me", WithCORS(HandleMe))

	// Admin
	http.HandleFunc("/api/admin/users", WithCORS(HandleAdminUsers))
	http.HandleFunc("/api/admin/promote", WithCORS(HandleAdminPromote))

	// Questions
	http.HandleFunc("/api/questions", WithCORS(HandleQuestions))
	http.HandleFunc("/api/questions/delete", WithCORS(HandleDeleteQuestion))
	http.HandleFunc("/api/questions/visibility", WithCORS(HandleQuestionVisibility))

	// Submissions
	http.HandleFunc("/api/submissions", WithCORS(HandleSubmissions))
	http.HandleFunc("/api/submissions/delete", WithCORS(HandleDeleteSubmission))

	// Reviews
	http.HandleFunc("/api/reviews", WithCORS(HandleReviews))

	// Leaderboard
	http.HandleFunc("/api/leaderboard", WithCORS(HandleLeaderboard))

	// Code execution
	http.HandleFunc("/api/run", WithCORS(HandleRunCode))

	// AI Hints
	http.HandleFunc("/api/hint", WithCORS(HandleHint))
	http.HandleFunc("/api/hint/status", WithCORS(HandleHintStatus))

	// Social
	http.HandleFunc("/api/search", WithCORS(HandleSearch))
	http.HandleFunc("/api/follows", WithCORS(HandleFollows))
	http.HandleFunc("/api/notifications", WithCORS(HandleNotifications))

	// Contest notifications
	http.HandleFunc("/api/contest-notifications", WithCORS(HandleNotifications2))

	// ── Challenges ────────────────────────────────────────────────────────────
	http.HandleFunc("/api/challenges", WithCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			HandleCreateChallenge(w, r)
		} else {
			HandleListChallenges(w, r)
		}
	}))
	http.HandleFunc("/api/challenges/", WithCORS(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/accept"):
			HandleAcceptChallenge(w, r)
		case strings.HasSuffix(path, "/reject"):
			HandleRejectChallenge(w, r)
		case strings.HasSuffix(path, "/enter"):
			HandleChallengeEnter(w, r)
		case strings.HasSuffix(path, "/arena-submit"):
			HandleChallengeArenaSubmit(w, r)
		case strings.HasSuffix(path, "/result"):
			HandleChallengeResult(w, r)
		case strings.HasSuffix(path, "/violation"):
			HandleChallengeViolation(w, r)
		default:
			HandleGetChallenge(w, r)
		}
	}))
	http.HandleFunc("/api/admin/challenges", WithCORS(HandleAdminListChallenges))
	http.HandleFunc("/api/admin/challenges/", WithCORS(HandleAssignChallengeQuestions))

	// ── Tournaments ───────────────────────────────────────────────────────────
	http.HandleFunc("/api/tournaments", WithCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			HandleCreateTournament(w, r)
		} else {
			HandleListTournaments(w, r)
		}
	}))
	http.HandleFunc("/api/tournaments/", WithCORS(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/join"):
			HandleJoinTournament(w, r)
		case strings.HasSuffix(path, "/leave"):
			HandleLeaveTournament(w, r)
		case strings.HasSuffix(path, "/enter"):
			HandleTournamentEnter(w, r)
		case strings.HasSuffix(path, "/arena-submit"):
			HandleTournamentArenaSubmit(w, r)
		case strings.HasSuffix(path, "/leaderboard"):
			HandleTournamentLeaderboard(w, r)
		case strings.HasSuffix(path, "/violation"):
			HandleTournamentViolation(w, r)
		default:
			HandleGetTournament(w, r)
		}
	}))
	http.HandleFunc("/api/admin/tournaments/", WithCORS(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/questions") {
			HandleAssignTournamentQuestions(w, r)
		} else if r.Method == http.MethodDelete {
			HandleDeleteTournament(w, r)
		} else {
			http.Error(w, "not found", 404)
		}
	}))

	log.Printf("Routes registered — server starting at %s", time.Now().Format(time.RFC3339))
}
