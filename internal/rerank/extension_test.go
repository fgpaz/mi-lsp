package rerank

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExecuteReordersAndAppendsMissingCandidates(t *testing.T) {
	command, args := helperCommand(t, "partial")
	outcome, err := Execute(context.Background(), Config{Command: command, Args: args, TopN: 1}, "sensitive query", testCandidates())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []int{2, 0, 1}
	if fmt.Sprint(outcome.Order) != fmt.Sprint(want) {
		t.Fatalf("order = %v, want %v", outcome.Order, want)
	}
	if !outcome.Applied[2] || outcome.Applied[0] || outcome.Applied[1] {
		t.Fatalf("applied = %#v, want only index 2", outcome.Applied)
	}
}

func TestExecuteResultsWithFiniteScores(t *testing.T) {
	command, args := helperCommand(t, "results")
	outcome, err := Execute(context.Background(), Config{Command: command, Args: args}, "query", testCandidates())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := fmt.Sprint(outcome.Order); got != "[1 0 2]" {
		t.Fatalf("order = %s, want [1 0 2]", got)
	}
}

func TestExecuteRejectsInvalidOutput(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "invalid json", mode: "invalid-json", want: "invalid_json"},
		{name: "duplicate index", mode: "duplicate", want: "duplicate_indices"},
		{name: "out of range index", mode: "out-of-range", want: "invalid_indices"},
		{name: "nonzero exit", mode: "exit", want: "command_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, args := helperCommand(t, tt.mode)
			_, err := Execute(context.Background(), Config{Command: command, Args: args}, "query", testCandidates())
			if err == nil {
				t.Fatalf("Execute succeeded, want %s", tt.want)
			}
			var safe *SafeError
			if !asSafeError(err, &safe) || safe.Kind != tt.want {
				t.Fatalf("err = %#v, want SafeError(%s)", err, tt.want)
			}
		})
	}
}

func TestResponseOrderRejectsNonFiniteScores(t *testing.T) {
	score := math.NaN()
	_, err := responseOrder(responseEnvelope{Results: []responseResult{{Index: 0, Score: &score}}})
	if err == nil {
		t.Fatalf("responseOrder succeeded, want invalid_score")
	}
	var safe *SafeError
	if !asSafeError(err, &safe) || safe.Kind != "invalid_score" {
		t.Fatalf("err = %#v, want SafeError(invalid_score)", err)
	}
}

func TestExecuteTimesOut(t *testing.T) {
	command, args := helperCommand(t, "sleep")
	_, err := Execute(context.Background(), Config{Command: command, Args: args, TimeoutMS: 10}, "query", testCandidates())
	if err == nil {
		t.Fatalf("Execute succeeded, want timeout")
	}
	var safe *SafeError
	if !asSafeError(err, &safe) || safe.Kind != "timeout" {
		t.Fatalf("err = %#v, want SafeError(timeout)", err)
	}
}

func TestExecuteBoundsSnippetsAndSuppressesWarnings(t *testing.T) {
	command, args := helperCommand(t, "inspect")
	outcome, err := Execute(context.Background(), Config{Command: command, Args: args, MaxSnippetChars: 4}, "secret query", []Candidate{
		{Archivo: "a.md", Snippet: "secret-snippet", Score: 1},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.Join(outcome.Warnings, " "); strings.Contains(got, "secret") || got == "" {
		t.Fatalf("warnings = %q, want sanitized non-empty warning", got)
	}
}

func testCandidates() []Candidate {
	return []Candidate{
		{Archivo: "a.md", Heading: "A", Snippet: "alpha secret", Score: 0.9},
		{Archivo: "b.md", Heading: "B", Snippet: "beta", Score: 0.8},
		{Archivo: "c.md", Heading: "C", Snippet: "gamma", Score: 0.7},
	}
}

func helperCommand(t *testing.T, mode string) (string, []string) {
	t.Helper()
	t.Setenv("MI_LSP_RERANK_HELPER", "1")
	return os.Args[0], []string{"-test.run=TestRerankHelperProcess", "--", mode}
}

func TestRerankHelperProcess(t *testing.T) {
	if os.Getenv("MI_LSP_RERANK_HELPER") != "1" {
		return
	}
	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}
	switch mode {
	case "partial":
		fmt.Println(`{"protocol_version":"mi-lsp-rerank-extension-v1","indices":[2]}`)
	case "results":
		fmt.Println(`{"protocol_version":"mi-lsp-rerank-extension-v1","results":[{"index":1,"score":0.99}]}`)
	case "invalid-json":
		fmt.Println(`not-json`)
	case "duplicate":
		fmt.Println(`{"indices":[1,1]}`)
	case "out-of-range":
		fmt.Println(`{"indices":[3]}`)
	case "exit":
		os.Exit(17)
	case "sleep":
		time.Sleep(500 * time.Millisecond)
		fmt.Println(`{"indices":[0]}`)
	case "inspect":
		var req requestEnvelope
		body, _ := io.ReadAll(os.Stdin)
		_ = json.Unmarshal(body, &req)
		if len(req.Candidates) != 1 || req.Candidates[0].Snippet != "secr" {
			fmt.Printf(`{"warnings":["bad snippet %s"]}`, req.Candidates[0].Snippet)
			return
		}
		fmt.Println(`{"indices":[0],"warnings":["secret query and secret-snippet must not leak"]}`)
	default:
		fmt.Println(`{"indices":[0]}`)
	}
	os.Exit(0)
}

func asSafeError(err error, target **SafeError) bool {
	if safe, ok := err.(*SafeError); ok {
		*target = safe
		return true
	}
	return false
}
