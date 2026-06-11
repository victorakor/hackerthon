package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if body.Name == "" || body.Email == "" || body.Password == "" {
		http.Error(w, "name, email and password required", 400)
		return
	}
	if len(body.Password) < 6 {
		http.Error(w, "password must be at least 6 characters", 400)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "server error", 500)
		return
	}
	var u User
	err = DB.QueryRow(
		`INSERT INTO users (name,email,password_hash) VALUES ($1,$2,$3) RETURNING id,name,email,is_admin,created_at`,
		body.Name, body.Email, string(hash)).
		Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			http.Error(w, "email already registered", 409)
			return
		}
		http.Error(w, "server error", 500)
		return
	}
	token := GenerateToken()
	DB.Exec(`INSERT INTO sessions (user_id,token,expires_at) VALUES ($1,$2,$3)`,
		u.ID, token, time.Now().Add(30*24*time.Hour))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{"user": u, "token": token})
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	var u User
	var hash string
	err := DB.QueryRow(
		`SELECT id,name,email,password_hash,is_admin,created_at FROM users WHERE email=$1`, body.Email).
		Scan(&u.ID, &u.Name, &u.Email, &hash, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		http.Error(w, "invalid email or password", 401)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		http.Error(w, "invalid email or password", 401)
		return
	}
	token := GenerateToken()
	DB.Exec(`INSERT INTO sessions (user_id,token,expires_at) VALUES ($1,$2,$3)`,
		u.ID, token, time.Now().Add(30*24*time.Hour))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"user": u, "token": token})
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		DB.Exec(`DELETE FROM sessions WHERE token=$1`, token)
	}
	w.WriteHeader(204)
}

func HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	var answered int
	DB.QueryRow(`SELECT COUNT(DISTINCT question_id) FROM submissions WHERE user_id=$1`, u.ID).Scan(&answered)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user":     u,
		"answered": answered,
	})
}
