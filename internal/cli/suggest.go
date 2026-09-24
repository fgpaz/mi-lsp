package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
)

const (
	suggestReasonRead      = "Read maps to one nav multi-read target"
	suggestReasonGrep      = "Grep pattern maps to nav search"
	suggestReasonGrepRegex = "Grep regex maps to nav search --regex"
	suggestReasonGlob      = "Glob symbol token maps to nav find"
)

type navSuggestItem struct {
	Tool    string   `json:"tool"`
	Command string   `json:"command"`
	Argv    []string `json:"argv"`
	Reason  string   `json:"reason"`
}

func newNavSuggestCommand(state *rootState) *cobra.Command {
	var tool string
	var argsJSON string
	command := &cobra.Command{
		Use:   "suggest",
		Short: "Map a Read, Grep, or Glob call to a local nav command",
		Long: `Suggest one local nav command for a raw Read, Grep, or Glob invocation.
No daemon, shell, or network. Exits 0 when there is no equivalent.
--args is a JSON object; Windows path backslashes are preserved.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := suggestNav(tool, argsJSON)
			if err != nil {
				return err
			}
			opts := state.queryOptions(cmd, "nav.suggest", nil)
			envelope := model.Envelope{
				Ok:        true,
				Backend:   "suggest",
				Operation: "nav.suggest",
				Items:     items,
			}
			envelope = output.ApplyEnvelopeLimits(envelope, opts)
			return state.printEnvelope(envelope, opts)
		},
	}
	command.Flags().StringVar(&tool, "tool", "", "Tool name: Read, Grep, or Glob")
	command.Flags().StringVar(&argsJSON, "args", "", "JSON object of tool arguments")
	return command
}

func suggestNav(tool, argsJSON string) ([]navSuggestItem, error) {
	switch canonicalSuggestTool(tool) {
	case "Read", "Grep", "Glob":
	default:
		return []navSuggestItem{}, nil
	}
	args, err := parseSuggestArgs(argsJSON)
	if err != nil {
		return nil, err
	}
	var item navSuggestItem
	var ok bool
	switch canonicalSuggestTool(tool) {
	case "Read":
		item, ok = suggestRead(args)
	case "Grep":
		item, ok = suggestGrep(args)
	case "Glob":
		item, ok = suggestGlob(args)
	}
	if !ok {
		return []navSuggestItem{}, nil
	}
	return []navSuggestItem{item}, nil
}

func canonicalSuggestTool(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "read":
		return "Read"
	case "grep":
		return "Grep"
	case "glob":
		return "Glob"
	default:
		return ""
	}
}

func parseSuggestArgs(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return map[string]any{}, nil
	}
	repaired := preserveWindowsPathEscapes(trimmed)
	var payload any
	if err := json.Unmarshal([]byte(repaired), &payload); err != nil {
		return nil, fmt.Errorf("invalid --args JSON: %w", err)
	}
	object, ok := payload.(map[string]any)
	if !ok || object == nil {
		return map[string]any{}, nil
	}
	return object, nil
}

// preserveWindowsPathEscapes keeps literal backslashes in --args strings.
// encoding/json treats \r as CR, but harness argv often carries a Windows path
// such as C:\repos\... with a single backslash rather than a doubled escape.
func preserveWindowsPathEscapes(input string) string {
	var b strings.Builder
	b.Grow(len(input) + 8)
	inString := false
	for i := 0; i < len(input); i++ {
		c := input[i]
		if !inString {
			b.WriteByte(c)
			if c == '"' {
				inString = true
			}
			continue
		}
		if c != '\\' {
			b.WriteByte(c)
			if c == '"' {
				inString = false
			}
			continue
		}
		if i+1 >= len(input) {
			b.WriteString(`\\`)
			continue
		}
		next := input[i+1]
		switch next {
		case '\\', '"', '/':
			b.WriteByte('\\')
			b.WriteByte(next)
			i++
		case 'u':
			if i+5 < len(input) && isHexByte(input[i+2]) && isHexByte(input[i+3]) && isHexByte(input[i+4]) && isHexByte(input[i+5]) {
				b.WriteString(input[i : i+6])
				i += 5
			} else {
				b.WriteString(`\\`)
				b.WriteByte(next)
				i++
			}
		default:
			b.WriteString(`\\`)
			b.WriteByte(next)
			i++
		}
	}
	return b.String()
}

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func suggestRead(args map[string]any) (navSuggestItem, bool) {
	file := jsonString(args, "file_path", "path", "file")
	if file == "" || strings.ContainsAny(file, "\r\n") {
		return navSuggestItem{}, false
	}
	target := multiReadTarget(file, args)
	return navSuggestItem{
		Tool:    "Read",
		Command: "nav multi-read",
		Argv:    []string{"nav", "multi-read", target},
		Reason:  suggestReasonRead,
	}, true
}

func multiReadTarget(file string, args map[string]any) string {
	offset, hasOffset := jsonPositiveInt(args, "offset")
	limit, hasLimit := jsonPositiveInt(args, "limit")
	if !hasOffset && !hasLimit {
		return file
	}
	start := 1
	if hasOffset {
		start = offset
	}
	if !hasLimit {
		return fmt.Sprintf("%s:%d-%d", file, start, start)
	}
	end := start + limit - 1
	if end < start {
		end = start
	}
	return fmt.Sprintf("%s:%d-%d", file, start, end)
}

func suggestGrep(args map[string]any) (navSuggestItem, bool) {
	pattern := jsonString(args, "pattern", "query")
	if pattern == "" || strings.ContainsAny(pattern, "\r\n") {
		return navSuggestItem{}, false
	}
	argv := []string{"nav", "search"}
	reason := suggestReasonGrep
	if suggestArgsRegex(args) {
		argv = append(argv, "--regex")
		reason = suggestReasonGrepRegex
	}
	argv = append(argv, pattern)
	return navSuggestItem{
		Tool:    "Grep",
		Command: "nav search",
		Argv:    argv,
		Reason:  reason,
	}, true
}

func suggestArgsRegex(args map[string]any) bool {
	return jsonBoolTrue(args, "regex", "regexp", "is_regex", "isRegex", "use_regex", "useRegex")
}

func suggestGlob(args map[string]any) (navSuggestItem, bool) {
	pattern := jsonString(args, "pattern", "glob", "glob_pattern", "query")
	if !isSymbolToken(pattern) {
		return navSuggestItem{}, false
	}
	return navSuggestItem{
		Tool:    "Glob",
		Command: "nav find",
		Argv:    []string{"nav", "find", pattern},
		Reason:  suggestReasonGlob,
	}, true
}

func isSymbolToken(pattern string) bool {
	if pattern == "" {
		return false
	}
	for i, r := range pattern {
		if i == 0 {
			if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return false
			}
			continue
		}
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func jsonString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := args[key]
		if !ok || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return ""
}

func jsonBoolTrue(args map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := args[key].(type) {
		case bool:
			if value {
				return true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "true", "1", "yes":
				return true
			}
		case float64:
			if value != 0 {
				return true
			}
		}
	}
	return false
}

func jsonPositiveInt(args map[string]any, key string) (int, bool) {
	value, ok := args[key]
	if !ok || value == nil {
		return 0, false
	}
	var n int
	switch typed := value.(type) {
	case float64:
		if typed <= 0 || typed > 1_000_000_000 {
			return 0, false
		}
		n = int(typed)
	case int:
		n = typed
	case int64:
		if typed <= 0 || typed > 1_000_000_000 {
			return 0, false
		}
		n = int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed <= 0 || parsed > 1_000_000_000 {
			return 0, false
		}
		n = int(parsed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || parsed <= 0 {
			return 0, false
		}
		n = parsed
	default:
		return 0, false
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}
