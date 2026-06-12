package app

import (
	"log"
	"net/http"
	"os"
	"time"
)

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
	http.Handle("/", http.FileServer(http.Dir("static")))

	// Auth
	http.HandleFunc("/api/auth/register", WithCORS(HandleRegister))
	http.HandleFunc("/api/auth/login", WithCORS(HandleLogin))
	http.HandleFunc("/api/auth/logout", WithCORS(HandleLogout))
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
		WriteTimeout: 60 * time.Second, // longer for streaming hint responses
	}
	log.Fatal(srv.ListenAndServe())
}
