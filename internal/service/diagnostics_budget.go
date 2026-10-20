package service

import (
	"encoding/json"
	"fmt"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// Bound the complete diagnostic envelope, not only the number of findings.
// Errors are already first; trimming the tail cannot let noisy warnings evict
// important errors. Counts always describe the observed snapshot before limits.
func boundDiagnostics(env model.Envelope, opts model.QueryOptions) model.Envelope {
	budget := opts.MaxChars
	if opts.TokenBudget > 0 {
		tokenChars := 1 << 20
		if opts.TokenBudget < tokenChars/4 {
			tokenChars = opts.TokenBudget * 4
		}
		if budget <= 0 || tokenChars < budget {
			budget = tokenChars
		}
	}
	if budget <= 0 {
		budget = 16000
	}
	if budget > 1<<20 {
		budget = 1 << 20
	}
	items, _ := env.Items.([]model.Diagnostic)
	for {
		encoded, _ := json.Marshal(env)
		if len(encoded) <= budget {
			break
		}
		env.Truncated = true
		hint := "diagnostics output budget reached; raise --token-budget/--max-chars or request one file; counts precede truncation"
		env.NextHint = &hint
		if len(items) > 0 {
			items = items[:len(items)-1]
			env.Items = items
			env.Diagnostics.Returned = len(items)
			continue
		}
		// Metadata itself may exceed a very small budget. Aggregate omissions by
		// reason, retaining explicit incompleteness instead of erasing it.
		if len(env.Omissions) > 1 {
			counts := map[string]int{}
			for _, o := range env.Omissions {
				counts[o.ErrorCode]++
			}
			env.Omissions = []model.EnvelopeOmission{{ErrorCode: "output_budget", Reason: fmt.Sprintf("omission details compacted: %v; request one file for details", counts)}}
			continue
		}
		if len(env.Warnings) > 0 {
			env.Warnings = nil
			continue
		}
		// A valid minimal envelope cannot fit every tiny requested budget. Honest
		// truncation/next-hint stays, and the generic renderer owns the final cap.
		break
	}
	return env
}
