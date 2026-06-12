package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// generateResetToken creates a secure random hex token.
func generateResetToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// sendResetEmail sends the password-reset link via SMTP.
// Required env vars: SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS, APP_URL
func sendResetEmail(toEmail, token string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	appURL := os.Getenv("APP_URL")

	if host == "" || user == "" || pass == "" || appURL == "" {
		return fmt.Errorf("SMTP not configured")
	}
	if port == "" {
		port = "587"
	}

	link := appURL + "/reset-password?token=" + token
	subject := "Reset your Hackerthon password"
	body := "Hi,\r\n\r\n" +
		"Click the link below to set a new password. It expires in 1 hour.\r\n\r\n" +
		link + "\r\n\r\n" +
		"If you didn't request this, you can safely ignore this email.\r\n"

	msg := "From: " + user + "\r\n" +
		"To: " + toEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" + body

	auth := smtp.PlainAuth("", user, pass, host)
	return smtp.SendMail(host+":"+port, auth, user, []string{toEmail}, []byte(msg))
}

// POST /api/auth/forgot-password
// Body: { "email": "..." }
// Always returns 200 to avoid leaking whether an email exists.
func HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Email) == "" {
		http.Error(w, "email required", 400)
		return
	}
	email := strings.TrimSpace(strings.ToLower(body.Email))

	// Look up user — do not reveal if not found
	var userID int
	err := DB.QueryRow(`SELECT id FROM users WHERE LOWER(email)=$1`, email).Scan(&userID)
	if err != nil {
		// Email not found — return 200 silently
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "If that email is registered you'll receive a reset link shortly."})
		return
	}

	// Invalidate any existing unused tokens for this user
	DB.Exec(`UPDATE password_resets SET used=TRUE WHERE user_id=$1 AND used=FALSE`, userID)

	token := generateResetToken()
	DB.Exec(`INSERT INTO password_resets (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID, token, time.Now().Add(1*time.Hour))

	// Fire and forget — don't block the response on SMTP
	go func() {
		if err := sendResetEmail(email, token); err != nil {
			fmt.Println("reset email error:", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "If that email is registered you'll receive a reset link shortly."})
}

// POST /api/auth/reset-password
// Body: { "token": "...", "password": "..." }
func HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if strings.TrimSpace(body.Token) == "" {
		http.Error(w, "token required", 400)
		return
	}
	if len(body.Password) < 6 {
		http.Error(w, "password must be at least 6 characters", 400)
		return
	}

	// Validate token
	var resetID, userID int
	var expiresAt time.Time
	var used bool
	err := DB.QueryRow(`SELECT id, user_id, expires_at, used FROM password_resets WHERE token=$1`, body.Token).
		Scan(&resetID, &userID, &expiresAt, &used)
	if err != nil {
		http.Error(w, "invalid or expired reset link", 400)
		return
	}
	if used {
		http.Error(w, "this reset link has already been used", 400)
		return
	}
	if time.Now().After(expiresAt) {
		http.Error(w, "this reset link has expired — please request a new one", 400)
		return
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "server error", 500)
		return
	}

	// Update password and mark token used in a single transaction
	tx, err := DB.Begin()
	if err != nil {
		http.Error(w, "server error", 500)
		return
	}
	tx.Exec(`UPDATE users SET password_hash=$1 WHERE id=$2`, string(hash), userID)
	tx.Exec(`UPDATE password_resets SET used=TRUE WHERE id=$1`, resetID)
	// Invalidate all sessions so old password sessions are kicked out
	tx.Exec(`DELETE FROM sessions WHERE user_id=$1`, userID)
	if err := tx.Commit(); err != nil {
		http.Error(w, "server error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Password updated. You can now log in."})
}

// GET /api/auth/validate-reset-token?token=...
// Used by the frontend to check if a token is still valid before showing the form.
func HandleValidateResetToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", 400)
		return
	}
	var expiresAt time.Time
	var used bool
	err := DB.QueryRow(`SELECT expires_at, used FROM password_resets WHERE token=$1`, token).
		Scan(&expiresAt, &used)
	if err != nil || used || time.Now().After(expiresAt) {
		http.Error(w, "invalid or expired reset link", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"valid": true})
}
