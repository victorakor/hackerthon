package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ─── In-memory hint counter ───────────────────────────────────────────────────
// Key: "userID:questionID"  Value: number of hints used this session
// Lives in process memory — resets on server restart or explicit logout.

var (
	hintMu      sync.Mutex
	hintCounter = map[string]int{}
)

const maxHintsPerQuestion = 2

func hintKey(userID, questionID int) string {
	return fmt.Sprintf("%d:%d", userID, questionID)
}

// ResetHintCountsForUser is called from HandleLogout.
func ResetHintCountsForUser(userID int) {
	prefix := fmt.Sprintf("%d:", userID)
	hintMu.Lock()
	defer hintMu.Unlock()
	for k := range hintCounter {
		if strings.HasPrefix(k, prefix) {
			delete(hintCounter, k)
		}
	}
}

// ─── /api/hint handler ────────────────────────────────────────────────────────

func HandleHint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	var body struct {
		QuestionID          int    `json:"question_id"`
		QuestionTitle       string `json:"question_title"`
		QuestionDescription string `json:"question_description"`
		UserCode            string `json:"user_code"`
		Language            string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if body.QuestionID == 0 {
		http.Error(w, "question_id required", 400)
		return
	}

	// ── Check + increment hint count ────────────────────────────────────────
	key := hintKey(u.ID, body.QuestionID)
	hintMu.Lock()
	used := hintCounter[key]
	if used >= maxHintsPerQuestion {
		hintMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":     "hint_limit_reached",
			"message":   fmt.Sprintf("You've used all %d AI hints for this question. Hints reset when you log out and back in.", maxHintsPerQuestion),
			"used":      used,
			"limit":     maxHintsPerQuestion,
			"remaining": 0,
		})
		return
	}
	hintCounter[key] = used + 1
	remaining := maxHintsPerQuestion - (used + 1)
	hintMu.Unlock()

	// ── Build Groq request ───────────────────────────────────────────────────
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		http.Error(w, "AI hints are not configured on this server", 503)
		return
	}

	lang := body.Language
	if lang == "" {
		lang = "go"
	}

	// Truncate code to keep request small
	code := body.UserCode
	if len(code) > 1600 {
		code = code[:1600] + "\n... (truncated)"
	}
	if strings.TrimSpace(code) == "" {
		code = "(no code written yet)"
	}

	prompt := fmt.Sprintf(`You are a patient coding mentor. Give the student ONE short nudge (2-4 sentences) in the right direction — never write any code for them.

Challenge: %s
Description: %s

Student's current %s code:
%s

Give a short hint pointing out what concept or approach they should reconsider. Do NOT write any code. Do NOT solve it for them. Be encouraging.`,
		body.QuestionTitle,
		body.QuestionDescription,
		lang,
		code,
	)

	// Groq OpenAI-compatible API — llama3-8b-8192 is free, fast, no cold starts
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": "llama3-8b-8192",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  180,
		"temperature": 0.5,
	})

	req, err := http.NewRequestWithContext(r.Context(), "POST",
		"https://api.groq.com/openai/v1/chat/completions",
		bytes.NewReader(reqBody))
	if err != nil {
		http.Error(w, "failed to build AI request", 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("AI service unreachable: %v", err), 502)
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read AI response", 502)
		return
	}

	if resp.StatusCode != 200 {
		http.Error(w, fmt.Sprintf("AI service error (%d): %s", resp.StatusCode, string(respBytes)), 502)
		return
	}

	// Groq returns OpenAI-compatible format:
	// {"choices": [{"message": {"content": "..."}}]}
	var groqResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &groqResp); err != nil || len(groqResp.Choices) == 0 {
		http.Error(w, fmt.Sprintf("unexpected AI response format: %s", string(respBytes)), 502)
		return
	}

	hintText := strings.TrimSpace(groqResp.Choices[0].Message.Content)

	// Strip any accidental code blocks the model might produce
	if idx := strings.Index(hintText, "```"); idx != -1 {
		hintText = strings.TrimSpace(hintText[:idx])
	}

	// ── Respond as SSE ───────────────────────────────────────────────────────
	// Simulate word-by-word streaming from the full response for a progressive UI feel.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, canFlush := w.(http.Flusher)

	// Meta event — updates the hint badge immediately
	fmt.Fprintf(w, "event: meta\ndata: {\"remaining\":%d,\"used\":%d,\"limit\":%d}\n\n",
		remaining, used+1, maxHintsPerQuestion)
	if canFlush {
		flusher.Flush()
	}

	// Emit word by word
	words := strings.Fields(hintText)
	for i, word := range words {
		token := word
		if i > 0 {
			token = " " + word
		}
		tokenJSON, _ := json.Marshal(token)
		fmt.Fprintf(w, "event: token\ndata: %s\n\n", tokenJSON)
		if canFlush {
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	if canFlush {
		flusher.Flush()
	}
}

// ─── /api/hint/status ─────────────────────────────────────────────────────────

func HandleHintStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := RequireAuth(w, r)
	if u == nil {
		return
	}
	var qid int
	fmt.Sscanf(r.URL.Query().Get("question_id"), "%d", &qid)

	hintMu.Lock()
	used := hintCounter[hintKey(u.ID, qid)]
	hintMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"used":      used,
		"limit":     maxHintsPerQuestion,
		"remaining": maxHintsPerQuestion - used,
	})
}
