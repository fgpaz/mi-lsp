package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	Missed     = "missed"
	Acceptable = "acceptable"
	Unknown    = "unknown"
)

// ToolObservation is a sanitized tool call. PatternText is used only to
// derive PatternShape and is never retained by the aggregate.
type ToolObservation struct {
	Harness      string
	Tool         string
	PatternShape string
	Ext          string
	Repo         string
	Wiki         bool
	LargeRead    bool
}

type MissedPattern struct {
	Harness        string `json:"harness"`
	Tool           string `json:"tool"`
	PatternShape   string `json:"pattern_shape"`
	Repo           string `json:"repo"`
	Class          string `json:"class"`
	Count          int    `json:"count"`
	Recommendation string `json:"recommendation"`
}

// ClassifyToolCall decides whether a raw tool call could have been a mi-lsp nav.
func ClassifyToolCall(obs ToolObservation) string {
	tool := canonicalTool(obs.Tool)
	indexed := indexedExt(obs.Ext) || obs.Wiki
	switch tool {
	case "grep", "rg", "find", "glob":
		if obs.Ext == ".md" && !obs.Wiki && obs.PatternShape == "literal" {
			return Acceptable
		}
		if indexed {
			return Missed
		}
		if nonIndexedExt(obs.Ext) {
			return Acceptable
		}
		if obs.PatternShape == "literal" && obs.Ext == ".md" && !obs.Wiki {
			return Acceptable
		}
		return Unknown
	case "read":
		if obs.LargeRead && indexed {
			return Missed
		}
		if nonIndexedExt(obs.Ext) {
			return Acceptable
		}
		return Unknown
	case "bash":
		if obs.PatternShape == "shell-search" && indexed {
			return Missed
		}
		return Unknown
	default:
		return Unknown
	}
}

func canonicalTool(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "grep", "rg":
		return "grep"
	case "glob":
		return "glob"
	case "read":
		return "read"
	case "bash", "powershell":
		return "bash"
	case "find":
		return "find"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func indexedExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".go", ".cs", ".ts", ".tsx", ".js", ".jsx", ".py", ".md":
		return true
	default:
		return false
	}
}

func nonIndexedExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".lock", ".sum", ".woff", ".pdf":
		return true
	default:
		return false
	}
}

func PatternShape(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "rg ") || strings.Contains(lower, "grep ") || strings.Contains(lower, "findstr ") {
		return "shell-search"
	}
	if strings.ContainsAny(value, "*?[") || strings.Contains(value, ".*") {
		return "glob-or-regex"
	}
	if strings.Contains(value, "/") || strings.Contains(value, `\`) {
		return "path"
	}
	if isSymbolShape(value) {
		return "symbol"
	}
	return "literal"
}

func isSymbolShape(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for _, r := range value {
		if r != '_' && r != '.' && (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func repeatedScan(tool, shape string) bool {
	switch canonicalTool(tool) {
	case "grep", "find", "glob":
		return true
	case "bash":
		return shape == "shell-search" || shape == "glob-or-regex"
	default:
		return false
	}
}

func Recommend(class, tool, shape string) string {
	if class != Missed {
		return ""
	}
	switch canonicalTool(tool) {
	case "grep", "rg", "find", "bash":
		if shape == "symbol" {
			return "skill wording: prefer mi-lsp nav find for a symbol before Grep"
		}
		return "tool description: nav search covers code and wiki text; do not scan with rg first"
	case "glob":
		return "new operation is unnecessary; point Glob of source files at nav find"
	case "read":
		return "hook: a large Read of an indexed file should call nav multi-read or nav find"
	default:
		return "skill wording: prefer mi-lsp nav before a raw scan"
	}
}

// AggregateMisses counts observations. A shape repeated 3 or more times in the
// same harness, tool and repo is missed even when a single call was unknown.
func AggregateMisses(observations []ToolObservation) []MissedPattern {
	type key struct{ harness, tool, shape, repo string }
	counts := map[key]int{}
	classes := map[key]string{}
	for _, obs := range observations {
		k := key{obs.Harness, canonicalTool(obs.Tool), obs.PatternShape, obs.Repo}
		counts[k]++
		class := ClassifyToolCall(obs)
		if classes[k] == "" || class == Missed {
			classes[k] = class
		}
	}
	out := make([]MissedPattern, 0, len(counts))
	for k, count := range counts {
		class := classes[k]
		if class == Unknown && count >= 3 && repeatedScan(k.tool, k.shape) {
			class = Missed
		}
		out = append(out, MissedPattern{
			Harness: k.harness, Tool: k.tool, PatternShape: k.shape, Repo: k.repo,
			Class: class, Count: count, Recommendation: Recommend(class, k.tool, k.shape),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tool < out[j].Tool
	})
	return out
}

type TranscriptRoot struct {
	Harness string
	Path    string
}

func DiscoverTranscriptRoots(home string) []TranscriptRoot {
	candidates := []TranscriptRoot{
		{"claude-code", filepath.Join(home, ".claude", "projects")},
		{"pi", filepath.Join(home, ".pi", "agent", "sessions")},
		{"codex", filepath.Join(home, ".codex", "sessions")},
		{"grok", filepath.Join(home, ".grok", "sessions")},
	}
	found := make([]TranscriptRoot, 0, len(candidates))
	for _, candidate := range candidates {
		info, err := os.Stat(candidate.Path)
		if err == nil && info.IsDir() {
			found = append(found, candidate)
		}
	}
	return found
}

// ScanTranscripts reads JSONL modified since the cutoff. It keeps only
// aggregates' inputs and drops pattern text after shaping it.
func ScanTranscripts(roots []TranscriptRoot, since time.Time, maxFiles int) []ToolObservation {
	if maxFiles <= 0 {
		maxFiles = 120
	}
	var files []string
	for _, root := range roots {
		_ = filepath.WalkDir(root.Path, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
				return nil
			}
			info, statErr := entry.Info()
			if statErr != nil || info.ModTime().Before(since) {
				return nil
			}
			files = append(files, path+"\x00"+root.Harness)
			return nil
		})
	}
	sort.Strings(files)
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}
	observations := make([]ToolObservation, 0)
	for _, item := range files {
		parts := strings.SplitN(item, "\x00", 2)
		observations = append(observations, scanJSONL(parts[0], parts[1])...)
	}
	return observations
}

func scanJSONL(path, harness string) []ToolObservation {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64*1024)
	out := make([]ToolObservation, 0)
	var read int
	for read < 1_500_000 {
		line, err := reader.ReadBytes('\n')
		read += len(line)
		if len(line) > 0 {
			out = append(out, observationsFromLine(line, harness)...)
		}
		if err != nil {
			break
		}
	}
	return out
}

func observationsFromLine(line []byte, harness string) []ToolObservation {
	var payload map[string]any
	if json.Unmarshal(line, &payload) != nil {
		return nil
	}
	out := make([]ToolObservation, 0)
	walkToolUses(payload, func(name string, input map[string]any) {
		pattern := firstString(input, "pattern", "query", "command", "glob", "glob_pattern")
		pathValue := firstString(input, "file_path", "path", "file")
		ext := strings.ToLower(filepath.Ext(pathValue))
		obs := ToolObservation{
			Harness:      harness,
			Tool:         name,
			PatternShape: PatternShape(pattern),
			Ext:          ext,
			Repo:         repoFromPath(pathValue),
			Wiki:         strings.Contains(strings.ToLower(pathValue), "docs"+string(os.PathSeparator)+"wiki") || strings.Contains(strings.ToLower(pathValue), ".docs/wiki"),
			LargeRead:    canonicalTool(name) == "read" && (intFrom(input, "limit") == 0 || intFrom(input, "limit") > 200),
		}
		out = append(out, obs)
	})
	return out
}

func walkToolUses(node any, visit func(string, map[string]any)) {
	switch typed := node.(type) {
	case map[string]any:
		if typed["type"] == "tool_use" {
			name, _ := typed["name"].(string)
			input, _ := typed["input"].(map[string]any)
			if name != "" && input != nil {
				visit(name, input)
			}
		}
		for _, value := range typed {
			walkToolUses(value, visit)
		}
	case []any:
		for _, value := range typed {
			walkToolUses(value, visit)
		}
	}
}

func firstString(input map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func intFrom(input map[string]any, key string) int {
	switch value := input[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func repoFromPath(path string) string {
	lower := strings.ToLower(path)
	marker := strings.ToLower(string(os.PathSeparator) + "repos" + string(os.PathSeparator))
	idx := strings.Index(lower, marker)
	if idx < 0 {
		marker = "/repos/"
		idx = strings.Index(lower, marker)
	}
	if idx < 0 {
		return "unknown"
	}
	rest := path[idx+len(marker):]
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) >= 2 {
		return parts[1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "unknown"
}
