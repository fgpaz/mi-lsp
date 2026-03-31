package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func Render(env model.Envelope, format string, compress bool) ([]byte, error) {
	if format == "" {
		format = "compact"
	}
	switch strings.ToLower(format) {
	case "json":
		return json.MarshalIndent(env, "", "  ")
	case "text":
		return []byte(renderText(env)), nil
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

func renderText(env model.Envelope) string {
	lines := []string{
		fmt.Sprintf("ok=%t backend=%s workspace=%s", env.Ok, env.Backend, env.Workspace),
	}
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
	return strings.Join(lines, "\n")
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
