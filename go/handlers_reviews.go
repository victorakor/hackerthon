package app

import (
	"encoding/json"
	"net/http"
)

func HandleReviews(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sid := r.URL.Query().Get("submission_id")
		if sid == "" {
			http.Error(w, "submission_id required", 400)
			return
		}
		rows, err := DB.Query(`SELECT id,submission_id,COALESCE(user_id,0),reviewer_name,rating,comment,created_at FROM reviews WHERE submission_id=$1 ORDER BY created_at DESC`, sid)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var reviews []Review
		for rows.Next() {
			var rv Review
			rows.Scan(&rv.ID, &rv.SubmissionID, &rv.UserID, &rv.ReviewerName, &rv.Rating, &rv.Comment, &rv.CreatedAt)
			reviews = append(reviews, rv)
		}
		if reviews == nil {
			reviews = []Review{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reviews)

	case http.MethodPost:
		u := RequireAuth(w, r)
		if u == nil {
			return
		}
		var rv Review
		if err := json.NewDecoder(r.Body).Decode(&rv); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if rv.SubmissionID == 0 || rv.Rating < 1 || rv.Rating > 5 {
			http.Error(w, "submission_id and rating(1-5) required", 400)
			return
		}
		rv.ReviewerName = u.Name
		rv.UserID = u.ID
		err := DB.QueryRow(`INSERT INTO reviews (submission_id,user_id,reviewer_name,rating,comment) VALUES ($1,$2,$3,$4,$5) RETURNING id,created_at`,
			rv.SubmissionID, u.ID, u.Name, rv.Rating, rv.Comment).Scan(&rv.ID, &rv.CreatedAt)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(rv)

	default:
		http.Error(w, "method not allowed", 405)
	}
}
