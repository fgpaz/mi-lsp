package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"time"
)

const ProtocolVersion = "mi-lsp-rerank-extension-v1"

const (
	defaultTimeoutMS       = 2000
	defaultCandidateCount  = 50
	defaultMaxSnippetChars = 500
	maxOutputBytes         = 1 << 20
)

type Config struct {
	Command         string
	Args            []string
	TimeoutMS       int
	CandidateCount  int
	TopN            int
	MaxSnippetChars int
}

type Candidate struct {
	Index     int      `json:"index"`
	Archivo   string   `json:"archivo"`
	Heading   string   `json:"heading,omitempty"`
	Snippet   string   `json:"snippet,omitempty"`
	Score     float64  `json:"score"`
	StartLine int      `json:"start_line,omitempty"`
	EndLine   int      `json:"end_line,omitempty"`
	Why       []string `json:"why,omitempty"`
}

type Outcome struct {
	Order    []int
	Applied  map[int]bool
	Warnings []string
}

type SafeError struct {
	Kind string
}

func (e *SafeError) Error() string {
	if e == nil || e.Kind == "" {
		return "failed"
	}
	return e.Kind
}

type requestEnvelope struct {
	ProtocolVersion string            `json:"protocol_version"`
	Query           string            `json:"query"`
	TopN            int               `json:"top_n"`
	Candidates      []Candidate       `json:"candidates"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type responseEnvelope struct {
	ProtocolVersion string           `json:"protocol_version,omitempty"`
	Indices         []int            `json:"indices,omitempty"`
	Results         []responseResult `json:"results,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
}

type responseResult struct {
	Index int      `json:"index"`
	Score *float64 `json:"score,omitempty"`
}

func Execute(ctx context.Context, cfg Config, query string, candidates []Candidate) (Outcome, error) {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cfg.Command) == "" {
		return Outcome{}, safeError("missing_command")
	}
	if len(candidates) == 0 {
		return Outcome{}, nil
	}

	bounded := boundCandidates(candidates, cfg.CandidateCount, cfg.MaxSnippetChars)
	topN := cfg.TopN
	if topN <= 0 || topN > len(bounded) {
		topN = len(bounded)
	}
	req := requestEnvelope{
		ProtocolVersion: ProtocolVersion,
		Query:           query,
		TopN:            topN,
		Candidates:      bounded,
		Metadata: map[string]string{
			"source": "nav.recall",
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Outcome{}, safeError("invalid_request")
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutMS)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(runCtx, cfg.Command, cfg.Args...)
	cmd.Stdin = bytes.NewReader(body)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return Outcome{}, safeError("timeout")
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, exec.ErrDot) {
			return Outcome{}, safeError("start_failed")
		}
		return Outcome{}, safeError("command_failed")
	}
	if stdout.Len() == 0 {
		return Outcome{}, safeError("empty_output")
	}
	if stdout.Len() > maxOutputBytes {
		return Outcome{}, safeError("invalid_output")
	}

	var resp responseEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return Outcome{}, safeError("invalid_json")
	}
	if resp.ProtocolVersion != "" && resp.ProtocolVersion != ProtocolVersion {
		return Outcome{}, safeError("invalid_output")
	}

	order, err := responseOrder(resp)
	if err != nil {
		return Outcome{}, err
	}
	outcome, err := validateOrder(order, len(bounded))
	if err != nil {
		return Outcome{}, err
	}
	outcome.Warnings = scrubWarnings(resp.Warnings)
	return outcome, nil
}

func normalizeConfig(cfg Config) Config {
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = defaultTimeoutMS
	}
	if cfg.CandidateCount <= 0 {
		cfg.CandidateCount = defaultCandidateCount
	}
	if cfg.MaxSnippetChars <= 0 {
		cfg.MaxSnippetChars = defaultMaxSnippetChars
	}
	return cfg
}

func boundCandidates(candidates []Candidate, maxCandidates int, maxSnippetChars int) []Candidate {
	if maxCandidates > 0 && len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	bounded := make([]Candidate, len(candidates))
	for i, candidate := range candidates {
		candidate.Index = i
		candidate.Snippet = truncateRunes(candidate.Snippet, maxSnippetChars)
		bounded[i] = candidate
	}
	return bounded
}

func responseOrder(resp responseEnvelope) ([]int, error) {
	if len(resp.Results) > 0 {
		order := make([]int, 0, len(resp.Results))
		for _, result := range resp.Results {
			if result.Score != nil && (math.IsNaN(*result.Score) || math.IsInf(*result.Score, 0)) {
				return nil, safeError("invalid_score")
			}
			order = append(order, result.Index)
		}
		return order, nil
	}
	if len(resp.Indices) > 0 {
		return append([]int(nil), resp.Indices...), nil
	}
	return nil, safeError("empty_output")
}

func validateOrder(order []int, candidateCount int) (Outcome, error) {
	seen := make(map[int]bool, len(order))
	final := make([]int, 0, candidateCount)
	for _, index := range order {
		if index < 0 || index >= candidateCount {
			return Outcome{}, safeError("invalid_indices")
		}
		if seen[index] {
			return Outcome{}, safeError("duplicate_indices")
		}
		seen[index] = true
		final = append(final, index)
	}
	for index := 0; index < candidateCount; index++ {
		if !seen[index] {
			final = append(final, index)
		}
	}
	return Outcome{Order: final, Applied: seen}, nil
}

func scrubWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("rerank extension returned %d warning(s); details suppressed", len(warnings))}
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func safeError(kind string) error {
	return &SafeError{Kind: kind}
}
