package service

import (
	"context"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// graphReadSnapshot is the read-only query surface shared by canonical and
// layered views. Query algorithms must not assume the effective generation has
// been published into the canonical database or write a private worktree copy.
// The caller owns the pinned snapshot lifetime.
type graphReadSnapshot interface {
	Generation() (model.GraphGeneration, error)
	Close() error
	Node(context.Context, int) (model.GraphNodeRecord, error)
	NodesByIDs(context.Context, []int) (map[int]model.GraphNodeRecord, error)
	ResolveGraphSelector(context.Context, string) ([]model.GraphNodeRecord, string, error)
	ResolveGraphEdgeSelector(context.Context, string) ([]model.GraphEdgeRecord, error)
	Edges(context.Context, []int, string, []string, int) ([]model.GraphEdgeRecord, error)
	EvidenceRefs(context.Context, *int, *int, int) ([]string, error)
	EvidenceRefsByEdges(context.Context, []int, int) (map[int][]string, error)
	Stats(context.Context) (map[string]int, error)
	FacetStats(context.Context) (map[string]map[string]int, error)
	UnresolvedOmissions(context.Context, int) ([]model.EnvelopeOmission, error)
	Validate(context.Context) (model.GraphGeneration, error)
	SnapshotFreshness(string) model.GraphFreshness
	BoundedData(context.Context, int, int) (store.GraphBoundedData, error)
}

var _ graphReadSnapshot = (*store.GraphQuerySnapshot)(nil)
