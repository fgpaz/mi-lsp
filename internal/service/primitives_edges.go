package service

import (
	"context"
	"strconv"
	"sync"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/query"
)

func (a *App) qEdges(ctx context.Context, r model.CommandRequest, s query.Stage, items []model.QItem, wid string) qExecution {
	dir := firstArg(s)
	switch dir {
	case "refs", "callers", "callees", "impl":
	default:
		return qExecution{reason: "stage_failed"}
	}
	depth, _ := strconv.Atoi(s.Options["depth"])
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	if len(items) > 200 {
		items = items[:200]
	}
	results := make([]qExecution, len(items))
	jobs := make(chan int)
	workers := min(8, len(items))
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					results[index] = qExecution{reason: qContextCode(ctx.Err())}
					continue
				}
				source := items[index]
				payload := map[string]any{"symbol": source.Name, "selector": source.Name, "file": source.File, "line": source.Line, "depth": depth, "limit": 50}
				op := "nav.refs"
				if dir == "callers" {
					op = "nav.callers"
				}
				if dir == "callees" {
					op = "nav.callees"
				}
				if dir == "impl" {
					op = "nav.related"
				}
				env, err := a.qCall(ctx, r, op, payload)
				if err != nil {
					results[index] = qExecution{reason: qContextCodeOrStage(ctx, err)}
					continue
				}
				found := qItemsFromEnvelope(env, wid, "")
				if len(found) > 50 {
					found = found[:50]
				}
				for i := range found {
					found[i].Edge = &model.QEdge{Dir: dir, From: source.ID, Depth: depth}
				}
				results[index] = qExecution{items: found, reason: qEnvReason(env), fallback: env.FallbackUsed, generation: env.GenerationID, truncated: env.Truncated}
			}
		}()
	}
	for index := range items {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	var out []model.QItem
	reason, fallback := "", ""
	for _, result := range results {
		if result.reason != "" && reason == "" {
			reason = qPublicReason(result.reason)
		}
		if result.fallback != "" {
			fallback = result.fallback
		}
		for _, item := range result.items {
			if len(out) >= 200 {
				break
			}
			out = append(out, item)
		}
	}
	if ctx.Err() != nil {
		reason = qContextCode(ctx.Err())
	}
	return qExecution{items: out, reason: reason, fallback: fallback}
}

func qContextCodeOrStage(ctx context.Context, err error) string {
	if ctx != nil && ctx.Err() != nil {
		return qContextCode(ctx.Err())
	}
	if err != nil {
		return "stage_failed"
	}
	return ""
}
