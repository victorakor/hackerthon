package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

func GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func GetUserFromToken(r *http.Request) (*User, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, fmt.Errorf("no token")
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	var u User
	err := DB.QueryRow(`
		SELECT u.id, u.name, u.email, u.is_admin
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token=$1 AND s.expires_at > NOW()`, token).
		Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	return &u, nil
}

func RequireAuth(w http.ResponseWriter, r *http.Request) *User {
	u, err := GetUserFromToken(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return nil
	}
	return u
}

func RequireAdmin(w http.ResponseWriter, r *http.Request) *User {
	u := RequireAuth(w, r)
	if u == nil {
		return nil
	}
	if !u.IsAdmin {
		http.Error(w, "forbidden", 403)
		return nil
	}
	return u
}
