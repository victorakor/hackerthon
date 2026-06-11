package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ExecSemaphore limits concurrent code executions to protect server resources.
var ExecSemaphore = make(chan struct{}, 10)

// AcquireSemaphore tries to acquire a slot within 5 seconds, returns false if busy.
func AcquireSemaphore() bool {
	select {
	case ExecSemaphore <- struct{}{}:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

func RunCode(language, code, input string) (stdout string, runErr string) {
	if !AcquireSemaphore() {
		return "", "server busy — too many concurrent executions, try again in a moment"
	}
	defer func() { <-ExecSemaphore }()

	dir, err := os.MkdirTemp("", "run-*")
	if err != nil {
		return "", "failed to create temp dir"
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	switch language {
	case "go":
		f := filepath.Join(dir, "main.go")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			return "", "failed to write file"
		}
		cmd = exec.CommandContext(ctx, "go", "run", f)

	case "python":
		f := filepath.Join(dir, "main.py")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			return "", "failed to write file"
		}
		cmd = exec.CommandContext(ctx, "python3", f)

	case "javascript":
		f := filepath.Join(dir, "main.js")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			return "", "failed to write file"
		}
		cmd = exec.CommandContext(ctx, "node", "--max-old-space-size=64", f)

	case "bash":
		f := filepath.Join(dir, "main.sh")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			return "", "failed to write file"
		}
		cmd = exec.CommandContext(ctx, "bash", f)

	default:
		return "", fmt.Sprintf("language '%s' is not supported for test execution — submit without running to save anyway", language)
	}

	cmd.Stdin = bytes.NewBufferString(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	execErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return "", "time limit exceeded (10s)"
	}
	if execErr != nil {
		errMsg := strings.TrimSpace(errBuf.String())
		if errMsg == "" {
			errMsg = execErr.Error()
		}
		return "", errMsg
	}

	return strings.TrimSpace(outBuf.String()), ""
}

func RunTest(userCode, testFile string) RunResult {
	if !AcquireSemaphore() {
		return RunResult{
			Results: []TestResult{{Index: 1, Error: "server busy — too many concurrent executions, try again in a moment"}},
		}
	}
	defer func() { <-ExecSemaphore }()

	dir, err := os.MkdirTemp("", "gotest-*")
	if err != nil {
		return RunResult{Results: []TestResult{{Index: 1, Error: "failed to create temp dir"}}}
	}
	defer os.RemoveAll(dir)

	goMod := "module submission\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		return RunResult{Results: []TestResult{{Index: 1, Error: "failed to write go.mod"}}}
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(userCode), 0644); err != nil {
		return RunResult{Results: []TestResult{{Index: 1, Error: "failed to write main.go"}}}
	}
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(testFile), 0644); err != nil {
		return RunResult{Results: []TestResult{{Index: 1, Error: "failed to write main_test.go"}}}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-v", "./...")
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	cmd.Run() // top-level error ignored; we parse output instead

	if ctx.Err() == context.DeadlineExceeded {
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return RunResult{Results: []TestResult{{Index: 1, Error: "time limit exceeded (30s)"}}}
	}

	combined := outBuf.String()
	stderrOut := strings.TrimSpace(errBuf.String())

	if strings.TrimSpace(combined) == "" {
		if stderrOut == "" {
			stderrOut = "compilation failed (no output)"
		}
		return RunResult{Results: []TestResult{{Index: 1, Error: stderrOut}}}
	}

	return ParseGoTestOutput(combined, stderrOut)
}

func RunAgainstTestCases(language, code string, testCases []TestCase) RunResult {
	result := RunResult{Total: len(testCases)}
	var mu sync.Mutex
	var wg sync.WaitGroup

	results := make([]TestResult, len(testCases))

	for i, tc := range testCases {
		wg.Add(1)
		time.Sleep(time.Duration(i) * 50 * time.Millisecond)
		go func(idx int, tc TestCase) {
			defer wg.Done()
			got, runErr := RunCode(language, code, tc.Input)
			tr := TestResult{
				Index:    idx + 1,
				Input:    tc.Input,
				Expected: strings.TrimSpace(tc.Expected),
				Got:      got,
			}
			if runErr != "" {
				tr.Error = runErr
				tr.Passed = false
			} else {
				tr.Passed = strings.TrimSpace(got) == strings.TrimSpace(tc.Expected)
			}
			mu.Lock()
			results[idx] = tr
			mu.Unlock()
		}(i, tc)
	}
	wg.Wait()

	result.Results = results
	for _, r := range results {
		if r.Passed {
			result.Passed++
		}
	}
	result.AllPassed = result.Passed == result.Total && result.Total > 0
	return result
}

func HandleRunCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	u := RequireAuth(w, r)
	if u == nil {
		return
	}

	var body struct {
		QuestionID int    `json:"question_id"`
		Code       string `json:"code"`
		Language   string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if body.Code == "" {
		http.Error(w, "code required", 400)
		return
	}
	if body.Language == "" {
		body.Language = "go"
	}

	var testCasesJSON, testFile string
	err := DB.QueryRow(
		`SELECT COALESCE(test_cases,'[]'), COALESCE(test_file,'') FROM questions WHERE id=$1`,
		body.QuestionID,
	).Scan(&testCasesJSON, &testFile)
	if err != nil {
		http.Error(w, "question not found", 404)
		return
	}

	if body.Language == "go" && strings.TrimSpace(testFile) != "" {
		result := RunTest(body.Code, testFile)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	var testCases []TestCase
	if err := json.Unmarshal([]byte(testCasesJSON), &testCases); err == nil && len(testCases) > 0 {
		result := RunAgainstTestCases(body.Language, body.Code, testCases)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	out, runErr := RunCode(body.Language, body.Code, "")
	result := RunResult{Total: 0, Passed: 0, AllPassed: false}
	if runErr != "" {
		result.Results = []TestResult{{Index: 1, Error: runErr, Passed: false}}
	} else {
		result.Results = []TestResult{{
			Index:  1,
			Got:    out,
			Passed: false,
			Error:  "no test cases defined for this question — output shown above",
		}}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
