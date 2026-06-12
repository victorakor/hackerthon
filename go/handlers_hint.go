package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

// ─── In-memory hint counter ───────────────────────────────────────────────────
// Key: "userID:questionID"  Value: number of hints used this session
// This lives in process memory, so it resets automatically on server restart
// or when the user's session token is deleted (logout).
// We protect it with a mutex because HTTP handlers run concurrently.

var (
	hintMu      sync.Mutex
	hintCounter = map[string]int{}
)

const maxHintsPerQuestion = 2

func hintKey(userID, questionID int) string {
	return fmt.Sprintf("%d:%d", userID, questionID)
}

// ResetHintCountsForUser is called from HandleLogout so counts are cleared
// the moment the user logs out (not just on server restart).
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

	// ── Build Anthropic request ──────────────────────────────────────────────
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		http.Error(w, "AI hints are not configured on this server", 503)
		return
	}

	lang := body.Language
	if lang == "" {
		lang = "go"
	}

	// Truncate code to ~400 tokens worth of characters to keep costs low
	code := body.UserCode
	if len(code) > 1600 {
		code = code[:1600] + "\n... (truncated)"
	}
	if strings.TrimSpace(code) == "" {
		code = "(no code written yet)"
	}

	systemPrompt := `You are a patient coding mentor helping a student solve a programming challenge.

Rules you MUST follow:
1. NEVER write code for the student — not even a single line of solution code.
2. Give ONE concise nudge: 2–4 sentences maximum.
3. Focus on WHAT concept, approach, or mistake to reconsider — not HOW to fix it.
4. Be encouraging and specific to what you actually see in their code.
5. If no code has been written yet, suggest a starting approach without writing code.
6. Do not repeat the question back to the student.`

	userMessage := fmt.Sprintf(`Challenge: %s

Description: %s

Student's current %s code:
%s

Give me a short nudge in the right direction without writing any code for me.`,
		body.QuestionTitle,
		body.QuestionDescription,
		lang,
		code,
	)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":      "claude-sonnet-4-6",
		"max_tokens": 180,
		"stream":     true,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userMessage},
		},
	})

	req, err := http.NewRequestWithContext(r.Context(), "POST",
		"https://api.anthropic.com/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		http.Error(w, "failed to build AI request", 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "AI service unreachable", 502)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body2, _ := io.ReadAll(resp.Body)
		http.Error(w, fmt.Sprintf("AI service error: %s", string(body2)), 502)
		return
	}

	// ── Stream SSE back to the browser ──────────────────────────────────────
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering if present
	w.Header().Set("X-Hints-Remaining", fmt.Sprintf("%d", remaining))

	flusher, canFlush := w.(http.Flusher)

	// Send remaining count as first event so the UI can update the badge
	fmt.Fprintf(w, "event: meta\ndata: {\"remaining\":%d,\"used\":%d,\"limit\":%d}\n\n",
		remaining, used+1, maxHintsPerQuestion)
	if canFlush {
		flusher.Flush()
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if event.Type != "content_block_delta" || event.Delta.Type != "text_delta" {
			continue
		}

		// Forward as a plain SSE "token" event
		tokenJSON, _ := json.Marshal(event.Delta.Text)
		fmt.Fprintf(w, "event: token\ndata: %s\n\n", tokenJSON)
		if canFlush {
			flusher.Flush()
		}
	}

	// Signal the stream is done
	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	if canFlush {
		flusher.Flush()
	}
}

// ─── /api/hint/status ─────────────────────────────────────────────────────────
// GET /api/hint/status?question_id=N  → returns how many hints used / remaining

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
