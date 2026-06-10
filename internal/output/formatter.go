package output

import (
	"encoding/json"
	"fmt"
	"strings"

	toon "github.com/toon-format/toon-go"
	"gopkg.in/yaml.v3"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// ApplyProfile filters envelope fields based on the profile setting.
// Agent profile reduces token cost by omitting human-only telemetry and verbose fields.
func ApplyProfile(env model.Envelope) model.Envelope {
	if env.Profile != model.OutputProfileAgent {
		return env
	}
	// For agent profile: auto-compress and omit verbose fields
	env.Items = compactItems(env.Items, true) // Auto-compress for agent
	return env
}

func Render(env model.Envelope, format string, compress bool) ([]byte, error) {
	if format == "" {
		format = "compact"
	}
	// Apply profile-based filtering before rendering
	env = ApplyProfile(env)
	switch strings.ToLower(format) {
	case "json":
		return json.MarshalIndent(env, "", "  ")
	case "text":
		return []byte(renderText(env)), nil
	case "toon":
		compact := env
		compact.Items = compactItems(env.Items, compress)
		return renderTOON(compact)
	case "yaml":
		compact := env
		compact.Items = compactItems(env.Items, compress)
		return renderYAML(compact)
	default:
		compact := env
		compact.Items = compactItems(env.Items, compress)
		// Calculate token estimate first: roughly len(output) / 4 chars per token
		// We estimate the output size before marshaling
		compact.Stats.TokensEstimate = 100 // placeholder estimate
		rendered, err := json.Marshal(compact)
		if err != nil {
			return rendered, err
		}
		// Now update token estimate based on actual output size
		compact.Stats.TokensEstimate = (len(rendered) + 3) / 4
		return rendered, nil
	}
}

// envelopeToMap converts an Envelope to map[string]any via JSON roundtrip.
// This ensures consistent key naming (using json tags) regardless of Items type.
func envelopeToMap(env model.Envelope) (map[string]any, error) {
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func renderTOON(env model.Envelope) ([]byte, error) {
	m, err := envelopeToMap(env)
	if err != nil {
		return nil, err
	}
	if sanitizeTOONMap(m) {
		appendTOONWarning(m, "toon output sanitized unsafe control characters")
	}
	out, err := toon.Marshal(m)
	if err != nil {
		return nil, err
	}
	// Update token estimate in the map and re-marshal to include it
	if stats, ok := m["stats"].(map[string]any); ok {
		stats["tokens_est"] = (len(out) + 3) / 4
		out, err = toon.Marshal(m)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func sanitizeTOONMap(m map[string]any) bool {
	_, changed := sanitizeTOONValue(m)
	return changed
}

func sanitizeTOONValue(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			sanitized, childChanged := sanitizeTOONValue(child)
			if childChanged {
				typed[key] = sanitized
				changed = true
			}
		}
		return typed, changed
	case []any:
		changed := false
		for index, child := range typed {
			sanitized, childChanged := sanitizeTOONValue(child)
			if childChanged {
				typed[index] = sanitized
				changed = true
			}
		}
		return typed, changed
	case string:
		sanitized := escapeUnsafeTOONControls(typed)
		return sanitized, sanitized != typed
	default:
		return value, false
	}
}

func escapeUnsafeTOONControls(value string) string {
	var builder strings.Builder
	changed := false
	for _, r := range value {
		if isUnsafeTOONControl(r) {
			if !changed {
				builder.Grow(len(value) + 6)
				changed = true
			}
			_, _ = fmt.Fprintf(&builder, "\\u%04x", r)
			continue
		}
		if changed {
			builder.WriteRune(r)
		}
	}
	if !changed {
		return value
	}
	return builder.String()
}

func isUnsafeTOONControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return (r >= 0x00 && r < 0x20) || (r >= 0x7f && r <= 0x9f)
}

func appendTOONWarning(m map[string]any, warning string) {
	if existing, ok := m["warnings"]; ok {
		switch warnings := existing.(type) {
		case []any:
			for _, item := range warnings {
				if item == warning {
					return
				}
			}
			m["warnings"] = append(warnings, warning)
			return
		case []string:
			for _, item := range warnings {
				if item == warning {
					return
				}
			}
			m["warnings"] = append(warnings, warning)
			return
		}
	}
	m["warnings"] = []any{warning}
}

func renderYAML(env model.Envelope) ([]byte, error) {
	m, err := envelopeToMap(env)
	if err != nil {
		return nil, err
	}
	out, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}
	// Update token estimate in the map and re-marshal to include it
	if stats, ok := m["stats"].(map[string]any); ok {
		stats["tokens_est"] = (len(out) + 3) / 4
		out, err = yaml.Marshal(m)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func renderText(env model.Envelope) string {
	header := fmt.Sprintf("ok=%t backend=%s workspace=%s", env.Ok, env.Backend, env.Workspace)
	if strings.TrimSpace(env.Mode) != "" {
		header = fmt.Sprintf("ok=%t backend=%s mode=%s workspace=%s", env.Ok, env.Backend, env.Mode, env.Workspace)
	}
	lines := []string{header}
	switch items := env.Items.(type) {
	case []model.SymbolRecord:
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("%s %s %s:%d", item.Kind, item.Name, item.FilePath, item.StartLine))
		}
	case []model.ServiceSurfaceSummary:
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("service %s path=%s profile=%s endpoints=%d consumers=%d publishers=%d entities=%d", item.Service, item.Path, item.Profile, len(item.HTTPEndpoints), len(item.EventConsumers), len(item.EventPublishers), len(item.Entities)))
		}
	case []model.AskResult:
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("ask summary=%s primary=%s", item.Summary, item.PrimaryDoc.Path))
			for _, evidence := range item.CodeEvidence {
				lines = append(lines, fmt.Sprintf("  code %s %s:%d %s", evidence.Type, evidence.File, evidence.Line, evidence.Name))
			}
		}
	case []model.GovernanceStatus:
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("governance profile=%s base=%s sync=%s index=%s blocked=%t", item.Profile, item.EffectiveBase, item.Sync, item.IndexSync, item.Blocked))
			if item.AECanon.Status != "" {
				lines = append(lines, fmt.Sprintf("  ae_canon status=%s source=%s roots=%s blocking=%t reason=%s", item.AECanon.Status, item.AECanon.Source, strings.Join(item.AECanon.Roots, ","), item.AECanon.Blocking, item.AECanon.Reason))
			}
			if item.IndexSyncDetails != nil && item.IndexSyncDetails.Reason != "" {
				lines = append(lines, "  index_reason "+item.IndexSyncDetails.Reason)
			}
			for _, issue := range item.Issues {
				lines = append(lines, "  issue "+issue)
			}
		}
	case []model.VersionInfo:
		for _, item := range items {
			revision := item.VCSRevision
			if len(revision) > 12 {
				revision = revision[:12]
			}
			if revision == "" {
				revision = "unknown"
			}
			modified := item.VCSModified
			if modified == "" {
				modified = "unknown"
			}
			lines = append(lines, fmt.Sprintf("%s version=%s revision=%s modified=%s go=%s os=%s arch=%s protocol=%s rid=%s", item.Command, item.Version, revision, modified, item.GoVersion, item.GOOS, item.GOARCH, item.ProtocolVersion, item.WorkerRID))
		}
	case []map[string]any:
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("%v", item))
		}
	default:
		lines = append(lines, fmt.Sprintf("%v", env.Items))
	}
	if len(env.Warnings) > 0 {
		lines = append(lines, "warnings: "+strings.Join(env.Warnings, "; "))
	}
	if env.Error != nil {
		lines = append(lines, renderEnvelopeError(*env.Error))
	}
	for _, omission := range env.Omissions {
		lines = append(lines, renderEnvelopeOmission(omission))
	}
	if env.Hint != "" {
		lines = append(lines, "hint: "+env.Hint)
	}
	if env.NextHint != nil && strings.TrimSpace(*env.NextHint) != "" {
		lines = append(lines, "next_hint: "+strings.TrimSpace(*env.NextHint))
	}
	if env.Coach != nil {
		header := "coach: trigger=" + env.Coach.Trigger
		if env.Coach.Confidence != "" {
			header += " confidence=" + env.Coach.Confidence
		}
		lines = append(lines, header)
		if strings.TrimSpace(env.Coach.Message) != "" {
			lines = append(lines, "  "+strings.TrimSpace(env.Coach.Message))
		}
		for _, action := range env.Coach.Actions {
			lines = append(lines, fmt.Sprintf("  action %s %s -> %s", action.Kind, action.Label, action.Command))
		}
	}
	if env.Continuation != nil {
		lines = append(lines, "continuation: reason="+env.Continuation.Reason)
		lines = append(lines, "  next "+renderContinuationTarget(env.Continuation.Next))
		if env.Continuation.Alternate != nil {
			lines = append(lines, "  alternate "+renderContinuationTarget(*env.Continuation.Alternate))
		}
	}
	if env.MemoryPointer != nil {
		pointer := "memory_pointer:"
		if env.MemoryPointer.DocID != "" {
			pointer += " doc_id=" + env.MemoryPointer.DocID
		}
		if env.MemoryPointer.ReentryOp != "" {
			pointer += " reentry_op=" + env.MemoryPointer.ReentryOp
		}
		if env.MemoryPointer.Stale {
			pointer += " stale=true"
		}
		lines = append(lines, pointer)
		if strings.TrimSpace(env.MemoryPointer.Why) != "" {
			lines = append(lines, "  "+strings.TrimSpace(env.MemoryPointer.Why))
		}
		if strings.TrimSpace(env.MemoryPointer.Handoff) != "" {
			lines = append(lines, "  handoff "+strings.TrimSpace(env.MemoryPointer.Handoff))
		}
	}
	return strings.Join(lines, "\n")
}

func renderEnvelopeError(err model.EnvelopeError) string {
	parts := []string{"error:"}
	if err.Kind != "" {
		parts = append(parts, "kind="+err.Kind)
	}
	if err.Code != "" {
		parts = append(parts, "code="+err.Code)
	}
	if err.Stage != "" {
		parts = append(parts, "stage="+err.Stage)
	}
	if err.HintCode != "" {
		parts = append(parts, "hint_code="+err.HintCode)
	}
	if err.Retryable {
		parts = append(parts, "retryable=true")
	}
	if strings.TrimSpace(err.Message) != "" {
		parts = append(parts, "message="+strings.TrimSpace(err.Message))
	}
	return strings.Join(parts, " ")
}

func renderEnvelopeOmission(omission model.EnvelopeOmission) string {
	parts := []string{"omission:"}
	if omission.Input != "" {
		parts = append(parts, "input="+omission.Input)
	}
	if omission.Path != "" {
		parts = append(parts, "path="+omission.Path)
	}
	if omission.Reason != "" {
		parts = append(parts, "reason="+omission.Reason)
	}
	if omission.ErrorCode != "" {
		parts = append(parts, "error_code="+omission.ErrorCode)
	}
	if omission.RequestedRange != "" {
		parts = append(parts, "requested_range="+omission.RequestedRange)
	}
	return strings.Join(parts, " ")
}

func renderContinuationTarget(target model.ContinuationTarget) string {
	parts := []string{target.Op}
	if target.Query != "" {
		parts = append(parts, "query="+target.Query)
	}
	if target.DocID != "" {
		parts = append(parts, "doc_id="+target.DocID)
	}
	if target.Repo != "" {
		parts = append(parts, "repo="+target.Repo)
	}
	if target.Path != "" {
		parts = append(parts, "path="+target.Path)
	}
	if target.Symbol != "" {
		parts = append(parts, "symbol="+target.Symbol)
	}
	if target.Full {
		parts = append(parts, "full=true")
	}
	return strings.Join(parts, " ")
}

func compactItems(items any, compress bool) any {
	switch typed := items.(type) {
	case []model.SymbolRecord:
		compact := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry := map[string]any{
				"n":   item.Name,
				"k":   item.Kind,
				"f":   item.FilePath,
				"l":   item.StartLine,
				"sig": truncateSignature(item.Signature, 120),
			}
			// Only add optional fields if not in compress mode
			if !compress {
				entry["impl"] = item.Implements
				entry["sc"] = item.Scope
			}
			if item.Workspace != "" {
				entry["workspace"] = item.Workspace
			}
			compact = append(compact, entry)
		}
		return compact
	case []model.ServiceSurfaceSummary:
		compact := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			compact = append(compact, map[string]any{
				"service":           item.Service,
				"path":              item.Path,
				"profile":           item.Profile,
				"sources":           item.Sources,
				"symbols":           item.Symbols,
				"http_endpoints":    item.HTTPEndpoints,
				"event_consumers":   item.EventConsumers,
				"event_publishers":  item.EventPublishers,
				"entities":          item.Entities,
				"infrastructure":    item.Infrastructure,
				"archetype_matches": item.ArchetypeMatches,
				"next_queries":      item.NextQueries,
			})
		}
		return compact
	case []model.AskResult:
		compact := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			compact = append(compact, map[string]any{
				"summary":       item.Summary,
				"primary_doc":   item.PrimaryDoc,
				"doc_evidence":  item.DocEvidence,
				"code_evidence": item.CodeEvidence,
				"why":           item.Why,
				"next_queries":  item.NextQueries,
			})
		}
		return compact
	case []model.RecallResult:
		compact := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			compact = append(compact, map[string]any{
				"arch": item.Archivo,
				"h":    item.Heading,
				"s":    item.Score,
				"snip": item.Snippet,
				"l":    item.StartLine,
				"why":  item.Why,
			})
		}
		return compact
	case []model.GovernanceStatus:
		compact := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry := map[string]any{
				"profile":               item.Profile,
				"effective_base":        item.EffectiveBase,
				"effective_overlays":    item.EffectiveOverlays,
				"sync":                  item.Sync,
				"index_sync":            item.IndexSync,
				"ae_canon":              item.AECanon,
				"blocked":               item.Blocked,
				"issues":                item.Issues,
				"warnings":              item.Warnings,
				"human_doc":             item.HumanDoc,
				"projection_doc":        item.ProjectionDoc,
				"context_chain":         item.ContextChain,
				"closure_chain":         item.ClosureChain,
				"audit_chain":           item.AuditChain,
				"numbering_recommended": item.NumberingRecommended,
				"summary":               item.Summary,
			}
			// Only include detailed sync info and next steps if not in compress mode
			if !compress {
				entry["index_sync_details"] = item.IndexSyncDetails
				entry["next_steps"] = item.NextSteps
			}
			compact = append(compact, entry)
		}
		return compact
	case []map[string]any:
		compact := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry := map[string]any{}
			if value, ok := item["name"]; ok {
				entry["n"] = value
			}
			if value, ok := item["kind"]; ok {
				entry["k"] = value
			}
			if value, ok := item["file"]; ok {
				entry["f"] = value
			}
			if value, ok := item["line"]; ok {
				entry["l"] = value
			}
			if value, ok := item["signature"]; ok {
				entry["sig"] = truncateSignature(fmt.Sprintf("%v", value), 120)
			}
			if !compress {
				if value, ok := item["implements"]; ok {
					entry["impl"] = value
				}
				if value, ok := item["scope"]; ok {
					entry["sc"] = value
				}
			}
			if !compress {
				if value, ok := item["parent"]; ok {
					entry["p"] = value
				}
			}
			for key, value := range item {
				if _, exists := entry[key]; !exists && key != "name" && key != "kind" && key != "file" && key != "line" && key != "signature" && key != "implements" && key != "scope" && key != "parent" {
					entry[key] = value
				}
			}
			compact = append(compact, entry)
		}
		return compact
	default:
		return items
	}
}

// truncateSignature truncates a signature to maxLen chars, appending "..." if truncated.
func truncateSignature(sig string, maxLen int) string {
	if len(sig) <= maxLen {
		return sig
	}
	return sig[:maxLen] + "..."
}
