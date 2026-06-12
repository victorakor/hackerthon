package app

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// spaHandler serves static files when they exist, otherwise falls back to
// index.html so the frontend JS can handle client-side routes like /reset-password.
type spaHandler struct {
	fs http.Handler
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Let the real file server try first
	path := "static" + r.URL.Path
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// No matching file — serve index.html and let JS handle the route
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

	// Social — feed notifications (follows-based)
	http.HandleFunc("/api/search", WithCORS(HandleSearch))
	http.HandleFunc("/api/follows", WithCORS(HandleFollows))
	http.HandleFunc("/api/notifications", WithCORS(HandleNotifications))

	// Contest notifications (challenge/tournament system) — separate endpoint
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
		case strings.HasSuffix(path, "/submit"):
			HandleChallengeSubmit(w, r)
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
		case strings.HasSuffix(path, "/submit"):
			HandleTournamentSubmit(w, r)
		case strings.HasSuffix(path, "/leaderboard"):
			HandleTournamentLeaderboard(w, r)
		case strings.HasSuffix(path, "/violation"):
			HandleTournamentViolation(w, r)
		default:
			HandleGetTournament(w, r)
		}
	}))
	http.HandleFunc("/api/admin/tournaments/", WithCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			HandleDeleteTournament(w, r)
		} else {
			HandleAssignTournamentQuestions(w, r)
		}
	}))

	log.Println("✓ Routes registered")
}

func StartServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Server running at http://localhost:%s", port)
	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
