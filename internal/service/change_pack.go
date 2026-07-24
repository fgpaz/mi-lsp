package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// ChangePackPacket is a one-shot harness packet for PR/diff understanding:
// changed symbols, ranked affected surfaces, hub/community risk, and next reads.
type ChangePackPacket struct {
	Ref              string           `json:"ref,omitempty"`
	ChangedFiles     int              `json:"changed_files"`
	ChangedPaths     []string         `json:"changed_paths,omitempty"`
	ChangedSymbols   []DiffSymbol     `json:"changed_symbols,omitempty"`
	Affected         []AffectedItem   `json:"affected,omitempty"`
	ReadFirst        []FlowSliceRead  `json:"read_first,omitempty"`
	HubRisk          *HubRisk         `json:"hub_risk,omitempty"`
	WikiMustRead     []string         `json:"wiki_must_read,omitempty"`
	SuggestedTests   []string         `json:"suggested_tests,omitempty"`
	BatchNext        []batchOperation `json:"batch_next,omitempty"`
	GenerationID     string           `json:"generation_id,omitempty"`
	Determinism      string           `json:"determinism_digest,omitempty"`
	Backend          string           `json:"backend,omitempty"`
	ImpactFiles      int              `json:"impact_files,omitempty"`
	ImpactSymbols    int              `json:"impact_symbols,omitempty"`
}

func (a *App) changePack(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	ref := strings.TrimSpace(stringPayload(request.Payload, "ref"))
	if ref == "" {
		ref = strings.TrimSpace(stringPayload(request.Payload, "changed_ref"))
	}
	limit := intFromAny(request.Payload["limit"], 12)
	if limit <= 0 {
		limit = 12
	}
	if limit > 30 {
		limit = 30
	}

	warnings := []string{"change-pack is a harness packet; graph impact remains advisory when generation is missing"}
	packet := ChangePackPacket{Ref: ref}

	// 1) Diff context
	diffReq := request
	diffReq.Operation = "nav.diff-context"
	diffReq.Payload = cloneIntentPayload(request.Payload)
	if ref != "" {
		diffReq.Payload["ref"] = ref
	}
	diffEnv, diffErr := a.diffContext(ctx, diffReq)
	var diff *DiffContextResult
	if diffErr != nil {
		warnings = append(warnings, "diff-context unavailable: "+sanitizeIntentError(diffErr))
	} else if results, ok := diffEnv.Items.([]DiffContextResult); ok && len(results) > 0 {
		diff = &results[0]
		packet.ChangedFiles = diff.ChangedFiles
		packet.ChangedPaths = append([]string{}, diff.ChangedPaths...)
		if len(diff.ChangedSymbols) > limit {
			packet.ChangedSymbols = append([]DiffSymbol{}, diff.ChangedSymbols[:limit]...)
		} else {
			packet.ChangedSymbols = append([]DiffSymbol{}, diff.ChangedSymbols...)
		}
		if diff.Impact != nil {
			packet.ImpactFiles = diff.Impact.FilesAffected
			packet.ImpactSymbols = diff.Impact.SymbolsAffected
		}
		packet.GenerationID = diffEnv.GenerationID
		packet.Determinism = diffEnv.DeterminismDigest
		packet.Backend = diffEnv.Backend
		for _, warning := range diffEnv.Warnings {
			warnings = appendStringIfMissing(warnings, warning)
		}
	}

	// Explicit paths override / complement.
	explicitPaths := affectedPathsFromPayload(request.Payload["paths"])
	if len(explicitPaths) > 0 {
		packet.ChangedPaths = normalizeAffectedPaths(append(packet.ChangedPaths, explicitPaths...))
		packet.ChangedFiles = len(packet.ChangedPaths)
	}

	// 2) Affected with tests/docs
	affReq := request
	affReq.Operation = "nav.affected"
	affReq.Payload = cloneIntentPayload(request.Payload)
	if len(packet.ChangedPaths) > 0 {
		affReq.Payload["paths"] = packet.ChangedPaths
		affReq.Payload["from_git_diff"] = false
	} else {
		affReq.Payload["from_git_diff"] = true
		if ref != "" {
			affReq.Payload["changed_ref"] = ref
		}
	}
	affReq.Payload["include_tests"] = true
	affReq.Payload["include_docs"] = true
	if packet.GenerationID != "" {
		affReq.Payload["generation"] = packet.GenerationID
	}
	affEnv, affErr := a.affected(ctx, affReq)
	var affected []AffectedItem
	if affErr != nil {
		warnings = append(warnings, "affected unavailable: "+sanitizeIntentError(affErr))
	} else {
		if typed, ok := affEnv.Items.([]AffectedItem); ok {
			affected = typed
		}
		if packet.GenerationID == "" {
			packet.GenerationID = affEnv.GenerationID
		}
		if packet.Determinism == "" {
			packet.Determinism = affEnv.DeterminismDigest
		}
		if packet.Backend == "" {
			packet.Backend = affEnv.Backend
		} else if affEnv.Backend != "" && !strings.Contains(packet.Backend, affEnv.Backend) {
			packet.Backend = packet.Backend + "+" + affEnv.Backend
		}
		for _, warning := range affEnv.Warnings {
			warnings = appendStringIfMissing(warnings, warning)
		}
	}

	// 3) Hub / community risk
	db, dbErr := openWorkspaceDB(registration, "nav.change-pack", true)
	hubPaths := map[string]bool{}
	if dbErr != nil {
		warnings = append(warnings, "catalog unavailable for hub risk: "+sanitizeIntentError(dbErr))
	} else {
		defer db.Close()
		focusSymbols := make([]string, 0, len(packet.ChangedSymbols))
		for _, symbol := range packet.ChangedSymbols {
			focusSymbols = append(focusSymbols, symbol.Name)
		}
		risk, paths, hubWarnings := assessHubRisk(ctx, db, packet.ChangedPaths, focusSymbols)
		hubPaths = paths
		warnings = append(warnings, hubWarnings...)
		if risk.Warning != "" || len(risk.HubsTouched) > 0 || risk.CommunityOverlapRisk != "none" {
			packet.HubRisk = &risk
		}
	}

	// 4) Anti-noise ranking
	packet.ChangedPaths = rankPathsForHarness(packet.ChangedPaths, packet.ChangedPaths, hubPaths)
	if len(packet.ChangedPaths) > limit {
		packet.ChangedPaths = packet.ChangedPaths[:limit]
	}
	packet.Affected = rankAffectedForHarness(affected, packet.ChangedPaths, hubPaths)
	if len(packet.Affected) > limit {
		packet.Affected = packet.Affected[:limit]
	}

	// 5) Read-first + wiki + tests
	packet.ReadFirst = buildChangePackReads(packet, hubPaths, limit)
	wiki := buildIntentWikiPlan(packet.ChangedPaths, registration.Name)
	for _, item := range wiki.MustRead {
		packet.WikiMustRead = append(packet.WikiMustRead, item.Path)
	}
	for _, item := range packet.Affected {
		if item.Kind == "test" && item.SuggestedCommand != "" {
			packet.SuggestedTests = appendStringIfMissing(packet.SuggestedTests, item.SuggestedCommand)
		}
	}
	packet.BatchNext = buildChangePackBatchNext(packet)

	env := model.Envelope{
		Ok:                true,
		Workspace:         registration.Name,
		Backend:           firstNonEmpty(packet.Backend, "change-pack"),
		Mode:              "harness-packet",
		Items:             []ChangePackPacket{packet},
		Warnings:          dedupeStrings(warnings),
		GenerationID:      packet.GenerationID,
		DeterminismDigest: packet.Determinism,
		Stats: model.Stats{
			Ms:      time.Since(started).Milliseconds(),
			Files:   packet.ChangedFiles,
			Symbols: len(packet.ChangedSymbols),
		},
		Continuation: buildChangePackContinuation(packet),
	}
	if packet.HubRisk != nil && packet.HubRisk.Warning != "" {
		env.Hint = packet.HubRisk.Warning
	}
	return applyCoachPolicy(env, request.Context), nil
}

func buildChangePackReads(packet ChangePackPacket, hubPaths map[string]bool, limit int) []FlowSliceRead {
	focusSymbols := make([]string, 0, len(packet.ChangedSymbols))
	for _, symbol := range packet.ChangedSymbols {
		focusSymbols = append(focusSymbols, symbol.Name)
	}
	seen := map[string]struct{}{}
	out := make([]FlowSliceRead, 0, limit)

	add := func(path, symbol, why string, line int, base float64) {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || isNoisePath(path) {
			return
		}
		key := path + "|" + symbol + "|" + why
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		score := harnessUsefulnessScore(path, "function", symbol, base, packet.ChangedPaths, focusSymbols, hubPaths[path])
		out = append(out, FlowSliceRead{Path: path, Line: line, Symbol: symbol, Why: why, Score: score})
	}

	for _, symbol := range packet.ChangedSymbols {
		add(symbol.File, symbol.Name, "changed symbol", symbol.Line, 1.0)
	}
	for _, item := range packet.Affected {
		why := item.Reason
		if why == "" {
			why = "affected surface"
		}
		add(item.Path, filepath.Base(item.Path), why, 0, item.Confidence)
	}
	// Sort by score desc already partially ordered; re-rank.
	type pair struct {
		idx   int
		score float64
	}
	order := make([]pair, len(out))
	for i, item := range out {
		order[i] = pair{idx: i, score: item.Score}
	}
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			if order[j].score > order[i].score || (order[j].score == order[i].score && out[order[j].idx].Path < out[order[i].idx].Path) {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	ranked := make([]FlowSliceRead, 0, len(out))
	for _, entry := range order {
		ranked = append(ranked, out[entry.idx])
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

func buildChangePackBatchNext(packet ChangePackPacket) []batchOperation {
	ops := make([]batchOperation, 0, 3)
	if len(packet.ReadFirst) > 0 {
		ranges := make([]string, 0, 6)
		for _, read := range packet.ReadFirst {
			if read.Line > 0 {
				ranges = append(ranges, fmt.Sprintf("%s:%d-%d", read.Path, read.Line, read.Line+40))
			} else {
				ranges = append(ranges, read.Path)
			}
			if len(ranges) >= 6 {
				break
			}
		}
		ops = append(ops, batchOperation{
			ID: "multi-read",
			Op: "nav.multi-read",
			Params: map[string]any{
				"items": ranges,
			},
		})
	}
	if len(packet.ChangedPaths) > 0 {
		ops = append(ops, batchOperation{
			ID: "affected",
			Op: "nav.affected",
			Params: map[string]any{
				"paths":         packet.ChangedPaths,
				"include_tests": true,
				"include_docs":  true,
			},
		})
	}
	return ops
}

func buildChangePackContinuation(packet ChangePackPacket) *model.Continuation {
	if len(packet.BatchNext) == 0 {
		return nil
	}
	batch := make([]model.ContinuationBatchOp, 0, len(packet.BatchNext))
	for _, op := range packet.BatchNext {
		batch = append(batch, model.ContinuationBatchOp{
			ID:     op.ID,
			Op:     op.Op,
			Params: op.Params,
		})
	}
	return &model.Continuation{
		Reason: "change-pack ranked impact ready for one budgeted multi-op continuation",
		Next: model.ContinuationTarget{
			Op:    "nav.batch",
			Batch: batch,
		},
	}
}
