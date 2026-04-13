package service

import (
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	axiPreviewExpansionHint = "rerun with --full for expanded detail"
	axiPreviewSummaryHint   = "preview mode: rerun with --full for a wider first pass"
)

func isAXIMode(opts model.QueryOptions) bool {
	return opts.AXI
}

func isAXIPreview(opts model.QueryOptions) bool {
	return opts.AXI && !opts.Full
}

func applyAXIPreviewHints(env model.Envelope, opts model.QueryOptions, hint string) model.Envelope {
	if !isAXIPreview(opts) {
		return env
	}
	if strings.TrimSpace(env.Hint) == "" {
		if strings.TrimSpace(hint) == "" {
			env.Hint = axiPreviewSummaryHint
		} else {
			env.Hint = hint
		}
	}
	if env.NextHint == nil {
		next := axiPreviewExpansionHint
		env.NextHint = &next
	}
	return env
}

func trimAskResultForAXIPreview(result model.AskResult) model.AskResult {
	result.DocEvidence = trimSlice(result.DocEvidence, 2)
	result.CodeEvidence = trimSlice(result.CodeEvidence, 2)
	result.Why = trimSlice(result.Why, 3)
	result.NextQueries = trimSlice(result.NextQueries, 3)
	return result
}

func trimSlice[T any](items []T, limit int) []T {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]T(nil), items[:limit]...)
}

func buildWorkspaceAXINextSteps(alias string) []string {
	return []string{
		fmt.Sprintf("mi-lsp workspace status %s", alias),
		fmt.Sprintf("mi-lsp nav governance --workspace %s --format toon", alias),
		fmt.Sprintf("mi-lsp nav route %q --workspace %s --format toon", "how is this workspace organized?", alias),
		fmt.Sprintf("mi-lsp nav ask %q --workspace %s", "how is this workspace organized?", alias),
		fmt.Sprintf("mi-lsp nav workspace-map --workspace %s --axi --full", alias),
	}
}
