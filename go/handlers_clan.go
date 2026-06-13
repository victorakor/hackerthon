package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// ── Clan CRUD ─────────────────────────────────────────────────────────────────

// POST /api/clans
func HandleCreateClan(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	var body struct {
		Name        string `json:"name"`
		Tag         string `json:"tag"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Tag = strings.ToUpper(strings.TrimSpace(body.Tag))
	body.Description = strings.TrimSpace(body.Description)

	if body.Name == "" {
		http.Error(w, "name required", 400)
		return
	}
	if len(body.Tag) < 2 || len(body.Tag) > 5 {
		http.Error(w, "tag must be 2-5 characters", 400)
		return
	}

	// User must not already be in a clan
	var alreadyInClan bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_members WHERE user_id=$1)`, u.ID).Scan(&alreadyInClan)
	if alreadyInClan {
		http.Error(w, "you are already in a clan — leave it first", 409)
		return
	}

	// Insert clan
	var clanID int
	err := DB.QueryRow(`
		INSERT INTO clans (name, tag, description, created_by)
		VALUES ($1,$2,$3,$4) RETURNING id`,
		body.Name, body.Tag, body.Description, u.ID,
	).Scan(&clanID)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			http.Error(w, "clan name or tag already taken", 409)
			return
		}
		http.Error(w, "failed to create clan", 500)
		return
	}

	// Creator joins as clanhead immediately
	_, err = DB.Exec(`
		INSERT INTO clan_members (clan_id, user_id, role)
		VALUES ($1,$2,'clanhead')`, clanID, u.ID)
	if err != nil {
		http.Error(w, "failed to join clan after creation", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      clanID,
		"message": "Clan created",
	})
}

// GET /api/clans
func HandleListClans(w http.ResponseWriter, r *http.Request) {
	rows, err := DB.Query(`
		SELECT c.id, c.name, c.tag, c.description, c.rating, c.created_by, c.created_at,
		       COUNT(cm.user_id) AS member_count
		FROM clans c
		LEFT JOIN clan_members cm ON cm.clan_id = c.id
		GROUP BY c.id
		ORDER BY c.rating DESC, c.name ASC`)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	defer rows.Close()

	type ClanListItem struct {
		Clan
		MemberCount int `json:"member_count"`
	}

	var out []ClanListItem
	for rows.Next() {
		var item ClanListItem
		rows.Scan(
			&item.ID, &item.Name, &item.Tag, &item.Description,
			&item.Rating, &item.CreatedBy, &item.CreatedAt,
			&item.MemberCount,
		)
		out = append(out, item)
	}
	if out == nil {
		out = []ClanListItem{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// GET /api/clans/leaderboard  — clans sorted by rating (same as list but explicit)
func HandleClanLeaderboard(w http.ResponseWriter, r *http.Request) {
	HandleListClans(w, r)
}

// GET /api/clans/mine
func HandleMyClan(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	var clanID int
	err := DB.QueryRow(`SELECT clan_id FROM clan_members WHERE user_id=$1`, u.ID).Scan(&clanID)
	if err != nil {
		// Not in a clan — return null
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nil)
		return
	}

	detail, err := getClanDetail(clanID, u.ID)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// GET /api/clans/{id}
func HandleGetClan(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/clans/", "")
	if id == 0 {
		http.Error(w, "invalid clan id", 400)
		return
	}
	detail, err := getClanDetail(id, u.ID)
	if err != nil {
		http.Error(w, "clan not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// getClanDetail fetches full clan info + members, used by multiple handlers
func getClanDetail(clanID, requestingUserID int) (*ClanDetail, error) {
	var d ClanDetail
	err := DB.QueryRow(`
		SELECT id, name, tag, description, rating, created_by, created_at
		FROM clans WHERE id=$1`, clanID,
	).Scan(&d.ID, &d.Name, &d.Tag, &d.Description, &d.Rating, &d.CreatedBy, &d.CreatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := DB.Query(`
		SELECT cm.user_id, u.name, cm.role, cm.joined_at
		FROM clan_members cm
		JOIN users u ON u.id = cm.user_id
		WHERE cm.clan_id=$1
		ORDER BY
			CASE cm.role WHEN 'clanhead' THEN 0 WHEN 'general' THEN 1 ELSE 2 END,
			cm.joined_at ASC`, clanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var m ClanMember
		m.ClanID = clanID
		rows.Scan(&m.UserID, &m.UserName, &m.Role, &m.JoinedAt)
		if m.UserID == requestingUserID {
			d.MyRole = m.Role
		}
		d.Members = append(d.Members, m)
	}
	if d.Members == nil {
		d.Members = []ClanMember{}
	}
	d.MemberCount = len(d.Members)
	return &d, nil
}

// ── Membership ────────────────────────────────────────────────────────────────

// POST /api/clans/{id}/join
func HandleJoinClan(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/clans/", "/join")
	if id == 0 {
		http.Error(w, "invalid clan id", 400)
		return
	}

	// Check clan exists and has room
	var memberCount int
	err := DB.QueryRow(`
		SELECT COUNT(*) FROM clan_members WHERE clan_id=$1`, id).Scan(&memberCount)
	if err != nil {
		http.Error(w, "clan not found", 404)
		return
	}
	if memberCount >= 50 {
		http.Error(w, "clan is full (max 50 members)", 409)
		return
	}

	// Verify clan exists
	var exists bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clans WHERE id=$1)`, id).Scan(&exists)
	if !exists {
		http.Error(w, "clan not found", 404)
		return
	}

	// Leave current clan first if in one
	var currentClanID int
	DB.QueryRow(`SELECT clan_id FROM clan_members WHERE user_id=$1`, u.ID).Scan(&currentClanID)
	if currentClanID != 0 {
		if currentClanID == id {
			http.Error(w, "you are already in this clan", 409)
			return
		}
		leaveClan(currentClanID, u.ID)
	}

	// Join new clan as member
	_, err = DB.Exec(`
		INSERT INTO clan_members (clan_id, user_id, role)
		VALUES ($1,$2,'member')
		ON CONFLICT (user_id) DO UPDATE SET clan_id=$1, role='member', joined_at=NOW()`,
		id, u.ID)
	if err != nil {
		http.Error(w, "failed to join clan", 500)
		return
	}

	// Reassign roles based on leaderboard rating
	reassignClanRoles(id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "joined clan"})
}

// DELETE /api/clans/{id}/leave
func HandleLeaveClan(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/clans/", "/leave")
	if id == 0 {
		http.Error(w, "invalid clan id", 400)
		return
	}

	// Confirm user is in this clan
	var inClan bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_members WHERE clan_id=$1 AND user_id=$2)`,
		id, u.ID).Scan(&inClan)
	if !inClan {
		http.Error(w, "you are not in this clan", 403)
		return
	}

	leaveClan(id, u.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "left clan"})
}

// leaveClan removes a user, deletes clan if empty, reassigns roles otherwise
func leaveClan(clanID, userID int) {
	DB.Exec(`DELETE FROM clan_members WHERE clan_id=$1 AND user_id=$2`, clanID, userID)

	var remaining int
	DB.QueryRow(`SELECT COUNT(*) FROM clan_members WHERE clan_id=$1`, clanID).Scan(&remaining)

	if remaining == 0 {
		// Delete the clan entirely
		DB.Exec(`DELETE FROM clans WHERE id=$1`, clanID)
	} else {
		// Reassign roles among remaining members
		reassignClanRoles(clanID)
	}
}

// reassignClanRoles sorts members by their leaderboard avg_rating and sets
// clanhead (rank 1), general (rank 2), member (everyone else)
func reassignClanRoles(clanID int) {
	rows, err := DB.Query(`
		SELECT cm.user_id
		FROM clan_members cm
		LEFT JOIN (
			SELECT s.user_id, COALESCE(AVG(rv.rating), 0) AS avg_rating
			FROM submissions s
			LEFT JOIN reviews rv ON rv.submission_id = s.id
			GROUP BY s.user_id
		) lb ON lb.user_id = cm.user_id
		WHERE cm.clan_id = $1
		ORDER BY COALESCE(lb.avg_rating, 0) DESC, cm.joined_at ASC`, clanID)
	if err != nil {
		return
	}
	defer rows.Close()

	var members []int
	for rows.Next() {
		var uid int
		rows.Scan(&uid)
		members = append(members, uid)
	}

	for i, uid := range members {
		role := "member"
		switch i {
		case 0:
			role = "clanhead"
		case 1:
			role = "general"
		}
		DB.Exec(`UPDATE clan_members SET role=$1 WHERE clan_id=$2 AND user_id=$3`,
			role, clanID, uid)
	}
}

// ── Chat ──────────────────────────────────────────────────────────────────────

// GET /api/clans/{id}/chat?since={last_msg_id}
// since=0 (or absent) → full history (up to 200 messages) so new members see everything.
func HandleGetClanChat(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/clans/", "/chat")
	if id == 0 {
		http.Error(w, "invalid clan id", 400)
		return
	}

	// Must be a member
	var inClan bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_members WHERE clan_id=$1 AND user_id=$2)`,
		id, u.ID).Scan(&inClan)
	if !inClan {
		http.Error(w, "not a clan member", 403)
		return
	}

	var since int
	if s := r.URL.Query().Get("since"); s != "" {
		for _, c := range s {
			if c >= '0' && c <= '9' {
				since = since*10 + int(c-'0')
			}
		}
	}

	// since=0 means first load — fetch full history (last 200) so new members see all chats.
	// Incremental polls use LIMIT 100 to pick up only new messages since last poll.
	var rows *sql.Rows
	var err error
	if since == 0 {
		rows, err = DB.Query(`
			SELECT m.id, m.clan_id, m.user_id, u.name, cm.role, m.content,
			       m.reply_to, m.created_at
			FROM clan_messages m
			JOIN users u ON u.id = m.user_id
			JOIN clan_members cm ON cm.clan_id = m.clan_id AND cm.user_id = m.user_id
			WHERE m.clan_id=$1
			ORDER BY m.id ASC
			LIMIT 200`, id)
	} else {
		rows, err = DB.Query(`
			SELECT m.id, m.clan_id, m.user_id, u.name, cm.role, m.content,
			       m.reply_to, m.created_at
			FROM clan_messages m
			JOIN users u ON u.id = m.user_id
			JOIN clan_members cm ON cm.clan_id = m.clan_id AND cm.user_id = m.user_id
			WHERE m.clan_id=$1 AND m.id > $2
			ORDER BY m.id ASC
			LIMIT 100`, id, since)
	}
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	defer rows.Close()

	var msgs []ClanMessage
	for rows.Next() {
		var m ClanMessage
		rows.Scan(&m.ID, &m.ClanID, &m.UserID, &m.UserName, &m.Role, &m.Content,
			&m.ReplyTo, &m.CreatedAt)
		// Attach parent message snippet for reply threading
		if m.ReplyTo != nil {
			var parentUser, parentContent string
			DB.QueryRow(`
				SELECT u.name, m2.content
				FROM clan_messages m2
				JOIN users u ON u.id = m2.user_id
				WHERE m2.id=$1`, *m.ReplyTo).Scan(&parentUser, &parentContent)
			m.ReplyToUserName = parentUser
			if len(parentContent) > 120 {
				parentContent = parentContent[:120] + "…"
			}
			m.ReplyToContent = parentContent
		}
		m.Reactions = getMessageReactions(m.ID, u.ID)
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []ClanMessage{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

// POST /api/clans/{id}/chat

func HandleSendClanMessage(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	id := parseIDFromPath(r.URL.Path, "/api/clans/", "/chat")
	if id == 0 {
		http.Error(w, "invalid clan id", 400)
		return
	}

	// Must be a member
	var inClan bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_members WHERE clan_id=$1 AND user_id=$2)`,
		id, u.ID).Scan(&inClan)
	if !inClan {
		http.Error(w, "not a clan member", 403)
		return
	}

	var body struct {
		Content string `json:"content"`
		ReplyTo *int   `json:"reply_to,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" {
		http.Error(w, "content required", 400)
		return
	}
	if len(body.Content) > 2000 {
		http.Error(w, "message too long (max 2000 chars)", 400)
		return
	}

	// Validate reply_to if provided — must belong to the same clan
	if body.ReplyTo != nil {
		var exists bool
		DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_messages WHERE id=$1 AND clan_id=$2)`,
			*body.ReplyTo, id).Scan(&exists)
		if !exists {
			http.Error(w, "reply_to message not found in this clan", 400)
			return
		}
	}

	var msgID int
	err := DB.QueryRow(`
		INSERT INTO clan_messages (clan_id, user_id, content, reply_to)
		VALUES ($1,$2,$3,$4) RETURNING id`,
		id, u.ID, body.Content, body.ReplyTo,
	).Scan(&msgID)
	if err != nil {
		http.Error(w, "failed to send message", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         msgID,
		"created_at": time.Now().UTC(),
	})
}

// ── Reactions ─────────────────────────────────────────────────────────────────

// POST /api/clans/{id}/chat/{msgid}/react   body: {"emoji":"🔥"}
func HandleAddReaction(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	clanID, msgID := parseClanMsgIDs(r.URL.Path, "/react")
	if clanID == 0 || msgID == 0 {
		http.Error(w, "invalid path", 400)
		return
	}

	// Must be a member
	var inClan bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_members WHERE clan_id=$1 AND user_id=$2)`,
		clanID, u.ID).Scan(&inClan)
	if !inClan {
		http.Error(w, "not a clan member", 403)
		return
	}

	// Message must belong to this clan's chat
	var msgInClan bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_messages WHERE id=$1 AND clan_id=$2)`,
		msgID, clanID).Scan(&msgInClan)
	if !msgInClan {
		http.Error(w, "message not found in this clan", 404)
		return
	}

	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Emoji == "" {
		http.Error(w, "emoji required", 400)
		return
	}

	allowed := map[string]bool{"👍": true, "🔥": true, "💀": true, "✅": true, "🤝": true, "⚔️": true}
	if !allowed[body.Emoji] {
		http.Error(w, "emoji not allowed", 400)
		return
	}

	DB.Exec(`
		INSERT INTO clan_message_reactions (message_id, user_id, emoji)
		VALUES ($1,$2,$3)
		ON CONFLICT DO NOTHING`, msgID, u.ID, body.Emoji)

	reactions := getMessageReactions(msgID, u.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reactions)
}

// DELETE /api/clans/{id}/chat/{msgid}/react   body: {"emoji":"🔥"}
func HandleRemoveReaction(w http.ResponseWriter, r *http.Request) {
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	clanID, msgID := parseClanMsgIDs(r.URL.Path, "/react")
	if clanID == 0 || msgID == 0 {
		http.Error(w, "invalid path", 400)
		return
	}

	var inClan bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_members WHERE clan_id=$1 AND user_id=$2)`,
		clanID, u.ID).Scan(&inClan)
	if !inClan {
		http.Error(w, "not a clan member", 403)
		return
	}

	// Message must belong to this clan's chat
	var msgInClan bool
	DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM clan_messages WHERE id=$1 AND clan_id=$2)`,
		msgID, clanID).Scan(&msgInClan)
	if !msgInClan {
		http.Error(w, "message not found in this clan", 404)
		return
	}

	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Emoji == "" {
		http.Error(w, "emoji required", 400)
		return
	}

	DB.Exec(`DELETE FROM clan_message_reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3`,
		msgID, u.ID, body.Emoji)

	reactions := getMessageReactions(msgID, u.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reactions)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func getMessageReactions(messageID, requestingUserID int) []ClanMessageReaction {
	rows, err := DB.Query(`
		SELECT emoji,
		       COUNT(*) AS cnt,
		       BOOL_OR(user_id=$2) AS reacted
		FROM clan_message_reactions
		WHERE message_id=$1
		GROUP BY emoji
		ORDER BY emoji`, messageID, requestingUserID)
	if err != nil {
		return []ClanMessageReaction{}
	}
	defer rows.Close()

	var out []ClanMessageReaction
	for rows.Next() {
		var rx ClanMessageReaction
		rows.Scan(&rx.Emoji, &rx.Count, &rx.Reacted)
		out = append(out, rx)
	}
	if out == nil {
		return []ClanMessageReaction{}
	}
	return out
}

// parseClanMsgIDs extracts clanID and msgID from paths like
// /api/clans/3/chat/17/react
func parseClanMsgIDs(path, suffix string) (clanID, msgID int) {
	p := strings.TrimPrefix(path, "/api/clans/")
	if suffix != "" {
		p = strings.TrimSuffix(p, suffix)
	}
	// p is now "3/chat/17"
	parts := strings.Split(p, "/")
	if len(parts) != 3 {
		return 0, 0
	}
	for _, c := range parts[0] {
		if c >= '0' && c <= '9' {
			clanID = clanID*10 + int(c-'0')
		}
	}
	for _, c := range parts[2] {
		if c >= '0' && c <= '9' {
			msgID = msgID*10 + int(c-'0')
		}
	}
	return clanID, msgID
}
