package service

import (
	"context"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/reentry"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type loadedReentryMemory struct {
	Snapshot model.ReentryMemorySnapshot
	Pointer  *model.MemoryPointer
	Stale    bool
}

func loadReentryMemory(ctx context.Context, root string) (*loadedReentryMemory, error) {
	db, err := store.Open(root)
	if err != nil {
		return nil, nil
	}
	defer db.Close()

	snapshot, ok, err := store.LoadReentrySnapshot(ctx, db)
	if err != nil || !ok {
		return nil, err
	}
	stale := reentry.SnapshotStale(root, snapshot.SnapshotBuiltAt)
	return &loadedReentryMemory{
		Snapshot: snapshot,
		Pointer:  buildMemoryPointer(snapshot, stale),
		Stale:    stale,
	}, nil
}

func buildMemoryPointer(snapshot model.ReentryMemorySnapshot, stale bool) *model.MemoryPointer {
	if len(snapshot.RecentCanonicalChanges) == 0 && strings.TrimSpace(snapshot.Handoff) == "" {
		return nil
	}
	pointer := &model.MemoryPointer{
		Handoff: snapshot.Handoff,
		Stale:   stale,
	}
	if len(snapshot.RecentCanonicalChanges) > 0 {
		change := snapshot.RecentCanonicalChanges[0]
		pointer.DocID = firstNonEmpty(change.DocID, change.Path)
		pointer.Why = change.Why
		pointer.ReentryOp = change.Reentry.Op
	} else if snapshot.BestReentry.Op != "" {
		pointer.ReentryOp = snapshot.BestReentry.Op
	}
	return pointer
}

func buildWorkspaceStatusMemory(snapshot model.ReentryMemorySnapshot, stale bool) model.WorkspaceStatusMemory {
	return model.WorkspaceStatusMemory{
		SnapshotBuiltAt:        snapshot.SnapshotBuiltAt,
		Stale:                  stale,
		RecentCanonicalChanges: append([]model.ReentryMemoryChange(nil), snapshot.RecentCanonicalChanges...),
		Handoff:                snapshot.Handoff,
		BestReentry:            snapshot.BestReentry,
	}
}

func attachMemoryPointer(env model.Envelope, memory *loadedReentryMemory) model.Envelope {
	if memory != nil && memory.Pointer != nil {
		env.MemoryPointer = memory.Pointer
	}
	return env
}

func buildStatusContinuation(opts model.QueryOptions, memory *loadedReentryMemory) *model.Continuation {
	if isAXIPreview(opts) {
		next := model.ContinuationTarget{Op: "workspace.status", Full: true}
		continuation := &model.Continuation{Reason: "expand_preview", Next: next}
		if memory != nil && memory.Snapshot.BestReentry.Op != "" {
			alternate := memory.Snapshot.BestReentry
			continuation.Alternate = &alternate
		}
		return continuation
	}
	return buildMemoryFallbackContinuation(memory, false)
}

func buildSearchContinuation(pattern string, project model.ProjectFile, repoSelector string, items []map[string]any, memory *loadedReentryMemory) *model.Continuation {
	if repoSelector == "" && len(items) > 0 {
		if repo := searchSingleVisibleRepo(project, items); repo != "" {
			return &model.Continuation{
				Reason: "narrow_scope",
				Next: model.ContinuationTarget{
					Op:    "nav.search",
					Query: pattern,
					Repo:  repo,
				},
			}
		}
	}
	return buildMemoryFallbackContinuation(memory, true)
}

func buildAskContinuation(question string, project model.ProjectFile, result model.AskResult, warnings []string, opts model.QueryOptions, previewTrimmed bool, memory *loadedReentryMemory) *model.Continuation {
	if isAXIPreview(opts) && previewTrimmed {
		return &model.Continuation{
			Reason: "expand_preview",
			Next: model.ContinuationTarget{
				Op:    "nav.ask",
				Query: question,
				Full:  true,
			},
		}
	}
	if askCoachConfidence(result, warnings) == "low" {
		return &model.Continuation{
			Reason: "low_evidence",
			Next: model.ContinuationTarget{
				Op:    "nav.search",
				Query: bestAskCoachSearchQuery(question, result),
				Repo:  askRepoScope(project, result.CodeEvidence),
				DocID: strings.TrimSpace(result.PrimaryDoc.DocID),
				Path:  strings.TrimSpace(result.PrimaryDoc.Path),
			},
		}
	}
	if strings.TrimSpace(result.PrimaryDoc.Path) != "" {
		return &model.Continuation{
			Reason: "follow_doc",
			Next:   askDocSearchTarget(result.PrimaryDoc),
		}
	}
	return buildMemoryFallbackContinuation(memory, true)
}

func buildPackContinuation(task string, result model.PackResult, opts model.QueryOptions, memory *loadedReentryMemory) *model.Continuation {
	if !opts.Full {
		return &model.Continuation{
			Reason: "expand_preview",
			Next: model.ContinuationTarget{
				Op:    "nav.pack",
				Query: task,
				DocID: strings.TrimSpace(result.PrimaryDoc),
				Full:  true,
			},
		}
	}
	if len(result.Docs) > 0 {
		return &model.Continuation{
			Reason: "follow_doc",
			Next:   packDocSearchTarget(result.Docs[0]),
		}
	}
	return buildMemoryFallbackContinuation(memory, true)
}

func buildRouteContinuation(task string, result model.RouteResult, opts model.QueryOptions, memory *loadedReentryMemory) *model.Continuation {
	if !opts.Full {
		return &model.Continuation{
			Reason: "expand_preview",
			Next: model.ContinuationTarget{
				Op:    "nav.pack",
				Query: task,
				DocID: strings.TrimSpace(result.Canonical.AnchorDoc.DocID),
			},
		}
	}
	if strings.TrimSpace(result.Canonical.AnchorDoc.Path) != "" {
		return &model.Continuation{
			Reason: "follow_doc",
			Next:   routeDocSearchTarget(result.Canonical.AnchorDoc),
		}
	}
	return buildMemoryFallbackContinuation(memory, true)
}

func buildRelatedContinuation(symbol string, opts model.QueryOptions, memory *loadedReentryMemory) *model.Continuation {
	if !opts.Full {
		return &model.Continuation{
			Reason: "expand_preview",
			Next: model.ContinuationTarget{
				Op:     "nav.related",
				Symbol: symbol,
				Full:   true,
			},
		}
	}
	return buildMemoryFallbackContinuation(memory, true)
}

func buildWorkspaceMapContinuation(opts model.QueryOptions, memory *loadedReentryMemory) *model.Continuation {
	if isAXIPreview(opts) {
		return &model.Continuation{
			Reason: "expand_preview",
			Next: model.ContinuationTarget{
				Op:   "nav.workspace-map",
				Full: true,
			},
		}
	}
	return buildMemoryFallbackContinuation(memory, true)
}

func buildServiceContinuation(memory *loadedReentryMemory) *model.Continuation {
	return buildMemoryFallbackContinuation(memory, true)
}

func buildMemoryFallbackContinuation(memory *loadedReentryMemory, allowSnapshotReentry bool) *model.Continuation {
	if memory == nil {
		return nil
	}
	reason := "recent_change"
	if strings.TrimSpace(memory.Snapshot.Handoff) != "" {
		reason = "handoff_reentry"
	}
	if memory.Stale {
		next := model.ContinuationTarget{Op: "workspace.status", Full: true}
		continuation := &model.Continuation{Reason: reason, Next: next}
		if allowSnapshotReentry && memory.Snapshot.BestReentry.Op != "" {
			alternate := memory.Snapshot.BestReentry
			continuation.Alternate = &alternate
		}
		return continuation
	}
	if allowSnapshotReentry && memory.Snapshot.BestReentry.Op != "" {
		return &model.Continuation{Reason: reason, Next: memory.Snapshot.BestReentry}
	}
	return nil
}

func askDocSearchTarget(doc model.AskDocEvidence) model.ContinuationTarget {
	target := model.ContinuationTarget{Op: "nav.search", Path: strings.TrimSpace(doc.Path)}
	if strings.TrimSpace(doc.DocID) != "" {
		target.DocID = strings.TrimSpace(doc.DocID)
		target.Query = strings.TrimSpace(doc.DocID)
		return target
	}
	target.Query = firstNonEmpty(strings.TrimSpace(doc.Title), strings.TrimSpace(doc.Path))
	return target
}

func packDocSearchTarget(doc model.PackDoc) model.ContinuationTarget {
	target := model.ContinuationTarget{Op: "nav.search", Path: strings.TrimSpace(doc.Path)}
	if strings.TrimSpace(doc.DocID) != "" {
		target.DocID = strings.TrimSpace(doc.DocID)
		target.Query = strings.TrimSpace(doc.DocID)
		return target
	}
	target.Query = firstNonEmpty(strings.TrimSpace(doc.Title), strings.TrimSpace(doc.Path))
	return target
}

func routeDocSearchTarget(doc model.RouteDoc) model.ContinuationTarget {
	target := model.ContinuationTarget{Op: "nav.search", Path: strings.TrimSpace(doc.Path)}
	if strings.TrimSpace(doc.DocID) != "" {
		target.DocID = strings.TrimSpace(doc.DocID)
		target.Query = strings.TrimSpace(doc.DocID)
		return target
	}
	target.Query = firstNonEmpty(strings.TrimSpace(doc.Title), strings.TrimSpace(doc.Path))
	return target
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
