package app

import (
	"encoding/json"
	"net/http"
)

func HandleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	rows, err := DB.Query(`
		SELECT COALESCE(s.user_id,0), s.author_name,
		       COUNT(DISTINCT s.id) as submission_count,
		       COALESCE(AVG(rv.rating),0) as avg_rating,
		       COUNT(rv.id) as total_reviews
		FROM submissions s
		LEFT JOIN reviews rv ON rv.submission_id = s.id
		GROUP BY s.user_id, s.author_name
		ORDER BY avg_rating DESC, submission_count DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var board []Contributor
	for rows.Next() {
		var c Contributor
		rows.Scan(&c.UserID, &c.AuthorName, &c.SubmissionCount, &c.AvgRating, &c.TotalReviews)
		board = append(board, c)
	}
	if board == nil {
		board = []Contributor{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(board)
}

func HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if RequireAuth(w, r) == nil {
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name required", 400)
		return
	}

	type UserProfile struct {
		User        User         `json:"user"`
		Submissions []Submission `json:"submissions"`
	}

	rows, err := DB.Query(`SELECT id,name,email,is_admin,created_at FROM users WHERE LOWER(name) LIKE LOWER($1) ORDER BY name`, "%"+name+"%")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var results []UserProfile = []UserProfile{}

	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin, &u.CreatedAt); err != nil {
			continue
		}
		var subs []Submission
		func() {
			subRows, err := DB.Query(`
				SELECT s.id, s.question_id, COALESCE(s.user_id,0), s.author_name, s.code, s.language, s.notes, s.created_at,
				       COALESCE(AVG(rv.rating),0), COUNT(rv.id)
				FROM submissions s
				LEFT JOIN reviews rv ON rv.submission_id = s.id
				WHERE s.user_id = $1
				GROUP BY s.id ORDER BY s.created_at DESC`, u.ID)
			if err != nil {
				return
			}
			defer subRows.Close()
			for subRows.Next() {
				var s Submission
				subRows.Scan(&s.ID, &s.QuestionID, &s.UserID, &s.AuthorName, &s.Code, &s.Language, &s.Notes, &s.CreatedAt, &s.AvgRating, &s.ReviewCount)
				subs = append(subs, s)
			}
		}()
		if subs == nil {
			subs = []Submission{}
		}
		results = append(results, UserProfile{User: u, Submissions: subs})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func HandleFollows(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		u := RequireAuth(w, r)
		if u == nil {
			return
		}
		var body struct {
			FolloweeID int `json:"followee_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if body.FolloweeID == u.ID {
			http.Error(w, "cannot follow yourself", 400)
			return
		}
		DB.Exec(`INSERT INTO follows (follower_id,followee_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, u.ID, body.FolloweeID)
		w.WriteHeader(204)

	case http.MethodDelete:
		u := RequireAuth(w, r)
		if u == nil {
			return
		}
		var body struct {
			FolloweeID int `json:"followee_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		DB.Exec(`DELETE FROM follows WHERE follower_id=$1 AND followee_id=$2`, u.ID, body.FolloweeID)
		w.WriteHeader(204)

	case http.MethodGet:
		u := RequireAuth(w, r)
		if u == nil {
			return
		}
		rows, err := DB.Query(`
			SELECT u.id FROM follows f JOIN users u ON u.id = f.followee_id
			WHERE f.follower_id = $1`, u.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			rows.Scan(&id)
			ids = append(ids, id)
		}
		if ids == nil {
			ids = []int{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ids)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func HandleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	type Notification struct {
		Submission Submission `json:"submission"`
		Question   Question   `json:"question"`
	}
	rows, err := DB.Query(`
		SELECT s.id, s.question_id, COALESCE(s.user_id,0), s.author_name, s.code, s.language, s.notes, s.created_at,
		       COALESCE(AVG(rv.rating),0), COUNT(rv.id),
		       q.id, q.title, q.description, q.difficulty, q.category, q.hint_url, q.hint_text, q.visible
		FROM submissions s
		JOIN follows f ON f.followee_id = s.user_id
		JOIN questions q ON q.id = s.question_id
		LEFT JOIN reviews rv ON rv.submission_id = s.id
		WHERE f.follower_id = $1
		GROUP BY s.id, q.id
		ORDER BY s.created_at DESC
		LIMIT 50`, u.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var notifs []Notification
	for rows.Next() {
		var n Notification
		rows.Scan(
			&n.Submission.ID, &n.Submission.QuestionID, &n.Submission.UserID,
			&n.Submission.AuthorName, &n.Submission.Code, &n.Submission.Language,
			&n.Submission.Notes, &n.Submission.CreatedAt, &n.Submission.AvgRating, &n.Submission.ReviewCount,
			&n.Question.ID, &n.Question.Title, &n.Question.Description,
			&n.Question.Difficulty, &n.Question.Category, &n.Question.HintURL,
			&n.Question.HintText, &n.Question.Visible,
		)
		notifs = append(notifs, n)
	}
	if notifs == nil {
		notifs = []Notification{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notifs)
}
