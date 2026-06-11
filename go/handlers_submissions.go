package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

func HandleSubmissions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		qid := r.URL.Query().Get("question_id")
		var rows *sql.Rows
		var err error
		if qid != "" {
			rows, err = DB.Query(`
				SELECT s.id, s.question_id, COALESCE(s.user_id,0), s.author_name, s.code, s.language, s.notes, s.created_at,
				       COALESCE(AVG(rv.rating),0) as avg_rating,
				       COUNT(rv.id) as review_count
				FROM submissions s
				LEFT JOIN reviews rv ON rv.submission_id = s.id
				WHERE s.question_id = $1
				GROUP BY s.id
				ORDER BY avg_rating DESC, s.created_at ASC`, qid)
		} else {
			rows, err = DB.Query(`
				SELECT s.id, s.question_id, COALESCE(s.user_id,0), s.author_name, s.code, s.language, s.notes, s.created_at,
				       COALESCE(AVG(rv.rating),0) as avg_rating,
				       COUNT(rv.id) as review_count
				FROM submissions s
				LEFT JOIN reviews rv ON rv.submission_id = s.id
				GROUP BY s.id
				ORDER BY avg_rating DESC, s.created_at ASC`)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var subs []Submission
		for rows.Next() {
			var s Submission
			rows.Scan(&s.ID, &s.QuestionID, &s.UserID, &s.AuthorName, &s.Code, &s.Language, &s.Notes, &s.CreatedAt, &s.AvgRating, &s.ReviewCount)
			subs = append(subs, s)
		}
		if subs == nil {
			subs = []Submission{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(subs)

	case http.MethodPost:
		u := RequireAuth(w, r)
		if u == nil {
			return
		}
		var s Submission
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if s.QuestionID == 0 || s.Code == "" {
			http.Error(w, "question_id and code required", 400)
			return
		}
		s.AuthorName = u.Name
		s.UserID = u.ID
		if s.Language == "" {
			s.Language = "go"
		}
		err := DB.QueryRow(`INSERT INTO submissions (question_id,user_id,author_name,code,language,notes) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id,created_at`,
			s.QuestionID, u.ID, u.Name, s.Code, s.Language, s.Notes).Scan(&s.ID, &s.CreatedAt)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(s)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func HandleDeleteSubmission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	var p struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	var ownerID int
	DB.QueryRow(`SELECT COALESCE(user_id,0) FROM submissions WHERE id=$1`, p.ID).Scan(&ownerID)
	if !u.IsAdmin && ownerID != u.ID {
		http.Error(w, "forbidden", 403)
		return
	}
	res, err := DB.Exec(`DELETE FROM submissions WHERE id=$1`, p.ID)
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
