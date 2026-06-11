package app

import (
	"encoding/json"
	"net/http"
)

func HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if RequireAdmin(w, r) == nil {
			return
		}
		rows, err := DB.Query(`SELECT id,name,email,is_admin,created_at FROM users ORDER BY created_at`)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var users []User
		for rows.Next() {
			var u User
			rows.Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin, &u.CreatedAt)
			users = append(users, u)
		}
		if users == nil {
			users = []User{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)

	case http.MethodDelete:
		admin := RequireAdmin(w, r)
		if admin == nil {
			return
		}
		var body struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if body.ID == admin.ID {
			http.Error(w, "cannot delete yourself", 400)
			return
		}
		res, err := DB.Exec(`DELETE FROM users WHERE id=$1`, body.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			http.Error(w, "not found", 404)
			return
		}
		w.WriteHeader(204)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func HandleAdminPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if RequireAdmin(w, r) == nil {
		return
	}
	var body struct {
		ID      int  `json:"id"`
		IsAdmin bool `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	_, err := DB.Exec(`UPDATE users SET is_admin=$1 WHERE id=$2`, body.IsAdmin, body.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
