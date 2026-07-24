package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// FlowSlicePacket is a one-shot harness packet for understanding a code flow
// with minimal tokens: path + local neighborhood + ranked read targets.
type FlowSlicePacket struct {
	From           string              `json:"from,omitempty"`
	To             string              `json:"to,omitempty"`
	Selector       string              `json:"selector,omitempty"`
	Path           any                 `json:"path,omitempty"`
	Callers        any                 `json:"callers,omitempty"`
	Callees        any                 `json:"callees,omitempty"`
	Neighbors      any                 `json:"neighbors,omitempty"`
	ReadFirst      []FlowSliceRead     `json:"read_first,omitempty"`
	HubRisk        *HubRisk            `json:"hub_risk,omitempty"`
	BatchNext      []batchOperation    `json:"batch_next,omitempty"`
	GenerationID   string              `json:"generation_id,omitempty"`
	Determinism    string              `json:"determinism_digest,omitempty"`
	TokenHints     FlowSliceTokenHints `json:"token_hints,omitempty"`
}

type FlowSliceRead struct {
	Path   string  `json:"path"`
	Line   int     `json:"line,omitempty"`
	Symbol string  `json:"symbol,omitempty"`
	Why    string  `json:"why"`
	Score  float64 `json:"score,omitempty"`
}

type FlowSliceTokenHints struct {
	Profile     string `json:"profile,omitempty"`
	MaxItems    int    `json:"max_items,omitempty"`
	PreferMulti bool   `json:"prefer_multi_read,omitempty"`
}

func (a *App) flowSlice(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	from := strings.TrimSpace(stringPayload(request.Payload, "from"))
	to := strings.TrimSpace(stringPayload(request.Payload, "to"))
	selector := strings.TrimSpace(stringPayload(request.Payload, "selector"))
	if selector == "" {
		selector = strings.TrimSpace(stringPayload(request.Payload, "symbol"))
	}
	if from == "" && to == "" && selector == "" {
		return model.Envelope{}, fmt.Errorf("flow-slice requires --from/--to or a selector/symbol")
	}

	limit := intFromAny(request.Payload["limit"], 8)
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}

	warnings := []string{"flow-slice is a harness packet; graph claims remain generation-bound"}
	packet := FlowSlicePacket{
		From:     from,
		To:       to,
		Selector: selector,
		TokenHints: FlowSliceTokenHints{
			Profile:     "harness-micro",
			MaxItems:    limit,
			PreferMulti: true,
		},
	}

	db, dbErr := openWorkspaceDB(registration, "nav.flow-slice", true)
	if dbErr != nil {
		warnings = append(warnings, "catalog unavailable: "+sanitizeIntentError(dbErr))
	} else {
		defer db.Close()
	}

	focusSymbols := []string{}
	if selector != "" {
		focusSymbols = append(focusSymbols, selector)
	}
	if from != "" {
		focusSymbols = append(focusSymbols, from)
	}
	if to != "" {
		focusSymbols = append(focusSymbols, to)
	}

	var hubRisk HubRisk
	hubPaths := map[string]bool{}
	if db != nil {
		var hubWarnings []string
		hubRisk, hubPaths, hubWarnings = assessHubRisk(ctx, db, nil, focusSymbols)
		warnings = append(warnings, hubWarnings...)
		if hubRisk.Warning != "" {
			packet.HubRisk = &hubRisk
		} else if len(hubRisk.HubsTouched) > 0 {
			packet.HubRisk = &hubRisk
		}
	}

	// Path between endpoints when both present.
	if from != "" && to != "" {
		pathReq := request
		pathReq.Operation = "nav.path"
		pathReq.Payload = cloneIntentPayload(request.Payload)
		pathReq.Payload["from"] = from
		pathReq.Payload["to"] = to
		pathEnv, pathErr := a.graphQuery(ctx, pathReq)
		if pathErr != nil {
			warnings = append(warnings, "path unavailable: "+sanitizeIntentError(pathErr))
		} else {
			packet.Path = boundAnyItems(pathEnv.Items, limit)
			if pathEnv.GenerationID != "" {
				packet.GenerationID = pathEnv.GenerationID
			}
			if pathEnv.DeterminismDigest != "" {
				packet.Determinism = pathEnv.DeterminismDigest
			}
			for _, warning := range pathEnv.Warnings {
				warnings = appendStringIfMissing(warnings, warning)
			}
		}
	}

	// Neighborhood around primary selector (or from).
	primary := selector
	if primary == "" {
		primary = from
	}
	if primary != "" {
		for _, op := range []struct {
			name string
			set  func(any)
		}{
			{"nav.callers", func(v any) { packet.Callers = v }},
			{"nav.callees", func(v any) { packet.Callees = v }},
			{"nav.neighbors", func(v any) { packet.Neighbors = v }},
		} {
			child := request
			child.Operation = op.name
			child.Payload = cloneIntentPayload(request.Payload)
			child.Payload["selector"] = primary
			if packet.GenerationID != "" {
				child.Payload["generation"] = packet.GenerationID
			}
			env, opErr := a.graphQuery(ctx, child)
			if opErr != nil {
				warnings = append(warnings, op.name+" unavailable: "+sanitizeIntentError(opErr))
				continue
			}
			op.set(boundAnyItems(env.Items, limit))
			if packet.GenerationID == "" && env.GenerationID != "" {
				packet.GenerationID = env.GenerationID
			}
			for _, warning := range env.Warnings {
				warnings = appendStringIfMissing(warnings, warning)
			}
		}
	}

	packet.ReadFirst = buildFlowSliceReads(packet, focusSymbols, hubPaths, limit)
	packet.BatchNext = buildFlowSliceBatchNext(packet)

	env := model.Envelope{
		Ok:                true,
		Workspace:         registration.Name,
		Backend:           "flow-slice",
		Mode:              "harness-packet",
		Items:             []FlowSlicePacket{packet},
		Warnings:          dedupeStrings(warnings),
		GenerationID:      packet.GenerationID,
		DeterminismDigest: packet.Determinism,
		Stats:             model.Stats{Ms: time.Since(started).Milliseconds(), Files: len(packet.ReadFirst)},
		Continuation:      buildFlowSliceContinuation(packet),
	}
	return applyCoachPolicy(env, request.Context), nil
}

func boundAnyItems(items any, limit int) any {
	list := intentAnyItems(items)
	if len(list) > limit {
		return list[:limit]
	}
	return list
}

func buildFlowSliceReads(packet FlowSlicePacket, focusSymbols []string, hubPaths map[string]bool, limit int) []FlowSliceRead {
	type candidate struct {
		path   string
		line   int
		symbol string
		why    string
		score  float64
	}
	seen := map[string]struct{}{}
	candidates := make([]candidate, 0, 32)

	add := func(path, symbol, why string, line int, base float64) {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || isNoisePath(path) {
			return
		}
		key := path + "|" + symbol
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		score := harnessUsefulnessScore(path, "function", symbol, base, nil, focusSymbols, hubPaths[path])
		candidates = append(candidates, candidate{path: path, line: line, symbol: symbol, why: why, score: score})
	}

	harvest := func(value any, why string) {
		for _, item := range intentAnyItems(value) {
			switch typed := item.(type) {
			case map[string]any:
				path, _ := typed["path"].(string)
				if path == "" {
					path, _ = typed["file"].(string)
				}
				if path == "" {
					path, _ = typed["owner_path"].(string)
				}
				symbol, _ := typed["symbol"].(string)
				if symbol == "" {
					symbol, _ = typed["display"].(string)
				}
				if symbol == "" {
					symbol, _ = typed["name"].(string)
				}
				line := intFromAny(typed["line"], 0)
				add(path, symbol, why, line, 0.7)
			}
		}
	}

	harvest(packet.Path, "on shortest flow path")
	harvest(packet.Callers, "incoming caller on flow")
	harvest(packet.Callees, "outgoing callee on flow")
	harvest(packet.Neighbors, "local neighborhood node")

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].path < candidates[j].path
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]FlowSliceRead, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, FlowSliceRead{Path: c.path, Line: c.line, Symbol: c.symbol, Why: c.why, Score: c.score})
	}
	return out
}

func buildFlowSliceBatchNext(packet FlowSlicePacket) []batchOperation {
	if len(packet.ReadFirst) == 0 {
		return nil
	}
	ranges := make([]string, 0, len(packet.ReadFirst))
	for _, read := range packet.ReadFirst {
		if read.Line > 0 {
			start := read.Line
			end := read.Line + 40
			ranges = append(ranges, fmt.Sprintf("%s:%d-%d", read.Path, start, end))
		} else {
			ranges = append(ranges, read.Path)
		}
		if len(ranges) >= 6 {
			break
		}
	}
	ops := []batchOperation{{
		ID: "multi-read",
		Op: "nav.multi-read",
		Params: map[string]any{
			"items": ranges,
		},
	}}
	if packet.Selector != "" {
		ops = append(ops, batchOperation{
			ID: "related",
			Op: "nav.related",
			Params: map[string]any{
				"symbol": packet.Selector,
			},
		})
	}
	return ops
}

func buildFlowSliceContinuation(packet FlowSlicePacket) *model.Continuation {
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
		Reason: "flow-slice ranked anchors ready for one budgeted multi-op continuation",
		Next: model.ContinuationTarget{
			Op:    "nav.batch",
			Batch: batch,
		},
	}
}
