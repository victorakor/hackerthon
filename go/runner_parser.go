package app

import (
	"bufio"
	"strings"
)

func ParseGoTestOutput(output string) RunResult {
	type block struct {
		name    string
		passed  bool
		lines   []string
		decided bool
	}

	var blocks []*block
	byName := map[string]*block{}
	var current *block

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "=== RUN"):
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				name := parts[2]
				b := &block{name: name}
				blocks = append(blocks, b)
				byName[name] = b
				current = b
			}

		case strings.HasPrefix(line, "--- PASS:") || strings.HasPrefix(line, "--- FAIL:"):
			passed := strings.HasPrefix(line, "--- PASS:")
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := parts[2]
				if b, ok := byName[name]; ok {
					b.passed = passed
					b.decided = true
					current = nil
				}
			}

		default:
			if current != nil && !current.decided {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					current.lines = append(current.lines, trimmed)
				}
			}
		}
	}

	var results []TestResult
	index := 1
	for _, b := range blocks {
		if strings.Contains(b.name, "/") {
			continue
		}
		tr := TestResult{
			Index:  index,
			Passed: b.passed,
			Got:    b.name,
		}
		if !b.passed && len(b.lines) > 0 {
			tr.Error = strings.Join(b.lines, "\n")
		}
		results = append(results, tr)
		index++
	}

	if len(results) == 0 {
		for _, b := range blocks {
			tr := TestResult{
				Index:  index,
				Passed: b.passed,
				Got:    b.name,
			}
			if !b.passed && len(b.lines) > 0 {
				tr.Error = strings.Join(b.lines, "\n")
			}
			results = append(results, tr)
			index++
		}
	}

	if len(results) == 0 {
		return RunResult{
			Results: []TestResult{{Index: 1, Error: strings.TrimSpace(output)}},
		}
	}

	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	total := len(results)
	return RunResult{
		Passed:    passed,
		Total:     total,
		AllPassed: passed == total && total > 0,
		Results:   results,
	}
}
