package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

func HandleQuestions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		u, _ := GetUserFromToken(r)
		var rows *sql.Rows
		var err error
		if u != nil && u.IsAdmin {
			rows, err = DB.Query(`SELECT id,title,description,difficulty,category,hint_url,hint_text,visible,COALESCE(test_cases,'[]'),COALESCE(test_file,'') FROM questions ORDER BY id`)
		} else {
			rows, err = DB.Query(`SELECT id,title,description,difficulty,category,hint_url,hint_text,visible,COALESCE(test_cases,'[]'),COALESCE(test_file,'') FROM questions WHERE visible=TRUE ORDER BY id`)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var qs []Question
		for rows.Next() {
			var q Question
			rows.Scan(&q.ID, &q.Title, &q.Description, &q.Difficulty, &q.Category, &q.HintURL, &q.HintText, &q.Visible, &q.TestCases, &q.TestFile)
			qs = append(qs, q)
		}
		if qs == nil {
			qs = []Question{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qs)

	case http.MethodPost:
		if RequireAdmin(w, r) == nil {
			return
		}
		var q Question
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if q.Title == "" || q.Description == "" {
			http.Error(w, "title and description required", 400)
			return
		}
		if q.Difficulty == "" {
			q.Difficulty = "medium"
		}
		if q.Category == "" {
			q.Category = "General"
		}
		if q.HintURL == "" {
			q.HintURL = "https://pkg.go.dev"
		}
		if q.HintText == "" {
			q.HintText = "Go documentation"
		}
		if q.TestCases == "" {
			q.TestCases = "[]"
		}
		var tc []TestCase
		if err := json.Unmarshal([]byte(q.TestCases), &tc); err != nil {
			http.Error(w, "test_cases must be a valid JSON array", 400)
			return
		}
		err := DB.QueryRow(
			`INSERT INTO questions (title,description,difficulty,category,hint_url,hint_text,visible,test_cases,test_file)
			 VALUES ($1,$2,$3,$4,$5,$6,FALSE,$7,$8) RETURNING id`,
			q.Title, q.Description, q.Difficulty, q.Category, q.HintURL, q.HintText, q.TestCases, q.TestFile,
		).Scan(&q.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(q)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func HandleDeleteQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", 405)
		return
	}
	if RequireAdmin(w, r) == nil {
		return
	}
	var p struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	res, err := DB.Exec(`DELETE FROM questions WHERE id=$1`, p.ID)
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
}

func HandleQuestionVisibility(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", 405)
		return
	}
	if RequireAdmin(w, r) == nil {
		return
	}
	var body struct {
		ID      int  `json:"id"`
		Visible bool `json:"visible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	_, err := DB.Exec(`UPDATE questions SET visible=$1 WHERE id=$2`, body.Visible, body.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
