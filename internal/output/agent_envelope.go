package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const AgentMaxBytes = 8192
const AgentMaxItems = 20

// PrepareAgentEnvelope is a presentation boundary, not a second query engine.
// Native summaries remain the first item so incompleteness cannot be hidden by
// trimming matches. Human/full output retains the producer's original schema.
func PrepareAgentEnvelope(env model.Envelope, opts model.QueryOptions, command string) model.Envelope {
	if (opts.Operation != "find_symbol" && opts.Operation != "get_errors" && !strings.HasPrefix(opts.Operation, "nav.")) || env.ContractVersion == "q-v1" || env.Profile == model.OutputProfileHuman || opts.Full || opts.Verbose {
		return env
	}
	env.Coach = nil
	env.Metrics = nil
	env.MemoryPointer = nil
	env.DaemonPool = nil
	env.Stats = model.Stats{}
	env = commonItems(env)
	if len(env.Warnings) > 3 {
		count := len(env.Warnings)
		env.Warnings = append([]string(nil), env.Warnings[:3]...)
		env.Omissions = append(env.Omissions, model.EnvelopeOmission{ErrorCode: "output_warnings_budget", Reason: fmt.Sprintf("showing 3 of %d warnings; repeat the full command", count)})
		markAgentCut(&env, command, "output_warnings_budget")
	}
	// A condensed warning text is disclosed, but it is not a row or item cut, so
	// it neither sets output_budget nor marks the envelope truncated.
	condensedWarning := false
	for i, warning := range env.Warnings {
		if len(warning) > 512 {
			env.Warnings = append([]string(nil), env.Warnings...)
			env.Warnings[i] = string(prefixWithin([]byte(warning), 512))
			condensedWarning = true
		}
	}
	if condensedWarning && !hasOmissionCode(env.Omissions, "output_warning_condensed") {
		env.Omissions = append(env.Omissions, model.EnvelopeOmission{ErrorCode: "output_warning_condensed", Reason: "warning text condensed to 512 bytes; repeat the full command for the complete text"})
	}
	// output_budget means a snippet, row or item was really removed. A producer's
	// own output_budget omission is kept as the producer reported it.
	if hasTruncatedItem(env.Items) {
		markAgentTruncated(&env, command)
	}
	if env.Truncated || env.Degraded {
		env.Partial = true
	}
	if env.Partial && env.Reason == "" {
		env.Reason = "explicit_incomplete"
	}
	if env.Continuation != nil {
		continuation := *env.Continuation
		continuation.Next = model.ContinuationTarget{Command: continuationTargetCommand(continuation.Next, continuation.Cursor)}
		continuation.Alternate = nil
		continuation.Cursor = ""
		env.Continuation = &continuation
	} else if env.Partial || env.Truncated || env.Error != nil {
		env.Continuation = &model.Continuation{Reason: env.Reason, Next: model.ContinuationTarget{Command: command}}
	}
	if env.Error != nil && env.Hint == "" {
		env.Hint = "Revise error.code y omissions; corrija el selector o compruebe el backend con mi-lsp doctor y repita la consulta."
	}
	maxBytes := AgentMaxBytes
	if opts.MaxChars > 0 {
		maxBytes = opts.MaxChars
	} else if opts.TokenBudget > 0 && opts.TokenBudget*4 < maxBytes {
		maxBytes = opts.TokenBudget * 4
	}
	maxItems := opts.MaxItems
	if maxItems <= 0 {
		maxItems = AgentMaxItems
	}
	rows, _ := env.Items.([]map[string]any)
	if len(rows) > maxItems {
		rows = rows[:maxItems]
		env.Items = rows
		markAgentTruncated(&env, command)
	}
	for {
		encoded, err := json.Marshal(env)
		if err != nil || len(encoded)+1 <= maxBytes {
			break
		}
		// wiki.matrix carries a second, very large validation projection in
		// checks. Prefer bounded primary items over this auxiliary detail.
		if env.Checks != nil {
			env.Checks = nil
			env.Omissions = append(env.Omissions, model.EnvelopeOmission{ErrorCode: "output_checks_budget", Reason: "validation detail omitted; repeat the full command"})
			markAgentCut(&env, command, "output_checks_budget")
			continue
		}
		if len(rows) > 0 && !(len(rows) == 1 && rows[0]["result_kind"] == "summary") {
			rows = rows[:len(rows)-1]
			env.Items = rows
			markAgentTruncated(&env, command)
			continue
		}
		// Graph metadata can dominate even an empty result (e.g. wiki.matrix).
		// Disclose its omission and retain recovery, errors and partial status.
		if env.Nodes != nil || env.Edges != nil || env.Matrix != nil || env.WikiCodeContext != nil || env.Graph != nil || env.Roadmap != nil || env.Mermaid != nil || env.Unresolved != nil {
			env.Nodes = nil
			env.Edges = nil
			env.Matrix = nil
			env.WikiCodeContext = nil
			env.Graph = nil
			env.Roadmap = nil
			env.Mermaid = nil
			env.Unresolved = nil
			env.Omissions = append(env.Omissions, model.EnvelopeOmission{ErrorCode: "output_metadata_budget", Reason: "graph detail omitted; repeat the full command"})
			markAgentCut(&env, command, "output_metadata_budget")
			continue
		}
		// Never cut required error/omission metadata to satisfy an impossible budget.
		break
	}
	return env
}

// hasTruncatedItem reports whether commonItems cut a row snippet, which is the
// only presentation-side cut that does not already flag the envelope.
func hasTruncatedItem(items any) bool {
	rows, _ := items.([]map[string]any)
	for _, row := range rows {
		if cut, _ := row["truncated_item"].(bool); cut {
			return true
		}
	}
	return false
}

// markAgentCut records a truncation that removed a field rather than a row or
// item. Its reason names that field, so output_budget keeps meaning a real
// row, item or snippet cut.
func markAgentCut(env *model.Envelope, command string, reason string) {
	env.Truncated = true
	env.Partial = true
	if env.Reason == "" {
		env.Reason = reason
	}
	env.Continuation = &model.Continuation{Reason: reason, Next: model.ContinuationTarget{Command: command}}
	env.NextHint = nil
}

func markAgentTruncated(env *model.Envelope, command string) {
	env.Truncated = true
	env.Partial = true
	if env.Reason == "" {
		env.Reason = "output_budget"
	}
	if !hasOmissionCode(env.Omissions, "output_budget") {
		env.Omissions = append(env.Omissions, model.EnvelopeOmission{ErrorCode: "output_budget", Reason: "output bounded; repeat the full command for omitted detail"})
	}
	// A producer continuation may skip rows removed by this presentation cap.
	// Recovery therefore repeats the query with expanded budgets, not a fake page.
	env.Continuation = &model.Continuation{Reason: "output_budget", Next: model.ContinuationTarget{Command: command}}
	env.NextHint = nil
}

func commonItems(env model.Envelope) model.Envelope {
	raw, err := json.Marshal(env.Items)
	if err != nil {
		return env
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return env
	}
	rows := make([]map[string]any, 0)
	appendRow := func(value any, kind string) {
		row, ok := value.(map[string]any)
		if !ok {
			row = map[string]any{"value": value}
		}
		if kind != "" {
			row["result_kind"] = kind
		}
		normalizeItem(row)
		if snippet, ok := row["snippet"].(string); ok && len(snippet) > 1024 {
			row["snippet"] = string(prefixWithin([]byte(snippet), 1024))
			row["truncated_item"] = true
			if !hasOmissionCode(env.Omissions, "output_budget") {
				env.Omissions = append(env.Omissions, model.EnvelopeOmission{ErrorCode: "output_budget", Reason: "snippet preview limited to 1024 bytes; repeat the full command"})
			}
		}
		rows = append(rows, row)
	}
	switch value := value.(type) {
	case []any:
		for _, row := range value {
			if matrix, ok := row.(map[string]any); ok {
				if criteria, ok := matrix["criteria"].([]any); ok {
					delete(matrix, "criteria")
					// Matrix warnings already live in Envelope.Warnings.
					if len(env.Warnings) > 0 {
						delete(matrix, "warnings")
					}
					appendRow(matrix, "summary")
					for _, criterion := range criteria {
						appendRow(criterion, "criteria")
					}
					continue
				}
			}
			appendRow(row, "")
		}
	case map[string]any:
		// Native operations expose heterogeneous bounded sections. Flatten only
		// known match sections, preserving all safety/status fields in a summary.
		sections := []string{"definitions", "usages", "usage_candidates", "ranges", "diagnostics"}
		summary := make(map[string]any, len(value))
		for key, v := range value {
			summary[key] = v
		}
		for _, key := range sections {
			if _, ok := value[key].([]any); ok {
				delete(summary, key)
			}
		}
		delete(summary, "truncated")
		delete(summary, "next_hint")
		appendRow(summary, "summary")
		for _, key := range sections {
			if entries, ok := value[key].([]any); ok {
				for _, entry := range entries {
					appendRow(entry, key)
				}
			}
		}
	case nil:
	default:
		appendRow(value, "")
	}
	env.Items = rows
	return env
}

func normalizeItem(row map[string]any) {
	for _, key := range []string{"file_hash", "signature_hash", "search_text", "telemetry"} {
		delete(row, key)
	}
	aliases := []struct {
		key          string
		alternatives []string
		empty        any
	}{
		{"file", []string{"file_path", "path", "doc_path", "owner_path", "f"}, ""},
		{"line", []string{"start_line", "l"}, 0},
		{"symbol", []string{"qualified_name", "name", "display", "n"}, ""},
		{"snippet", []string{"content", "text", "signature", "sig", "snip"}, ""},
	}
	for _, alias := range aliases {
		if _, exists := row[alias.key]; !exists {
			row[alias.key] = alias.empty
			for _, key := range alias.alternatives {
				if value, exists := row[key]; exists && fmt.Sprint(value) != "" {
					row[alias.key] = value
					delete(row, key)
					break
				}
			}
		}
	}
}
