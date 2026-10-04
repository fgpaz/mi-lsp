package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type gitFrontierResult struct {
	Workspace        string            `json:"workspace"`
	GitCommit        *string           `json:"git_commit"`
	GitBase          *string           `json:"git_base"`
	GitFrontier      *string           `json:"git_frontier"`
	Generations      map[string]any    `json:"generations"`
	GenerationReason map[string]string `json:"generation_reasons,omitempty"`
	Coherence        string            `json:"coherence"`
	GraphFreshness   string            `json:"graph_freshness"`
	Unavailable      map[string]string `json:"unavailable,omitempty"`
}

func newGitFrontierResult(ctx context.Context, selector, callerCWD, baseSelector string) gitFrontierResult {
	incompatibleGraph := false
	result := gitFrontierResult{
		Workspace:        strings.TrimSpace(selector),
		Generations:      map[string]any{"docs": nil, "catalog": nil, "graph": nil},
		GenerationReason: map[string]string{},
		Coherence:        "degraded",
		GraphFreshness:   "unavailable",
		Unavailable:      map[string]string{},
	}
	resolution, err := workspace.ResolveWorkspaceSelectionReadOnly(selector, callerCWD)
	if err != nil {
		result.Unavailable["workspace"] = "workspace_resolution_failed"
		result.Unavailable["git_frontier"] = "workspace_unavailable"
		return result
	}
	result.Workspace = resolution.Registration.Name
	root := resolution.Registration.Root
	commit, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		result.Unavailable["git_commit"] = "git_head_unavailable"
	} else {
		result.GitCommit = stringPointer(commit)
	}
	base := strings.TrimSpace(baseSelector)
	if base == "" {
		base, err = gitOutput(ctx, root, "rev-parse", "--verify", "@{upstream}")
		if err != nil {
			base, err = gitOutput(ctx, root, "rev-parse", "--verify", "origin/main")
		}
	}
	if err != nil || base == "" {
		result.Unavailable["git_frontier"] = "git_base_unavailable"
	} else if baseSHA, resolveErr := gitOutput(ctx, root, "rev-parse", "--verify", base); resolveErr != nil {
		result.Unavailable["git_frontier"] = "git_base_unavailable"
	} else {
		result.GitBase = stringPointer(baseSHA)
		if commit != "" {
			frontier, frontierErr := calculateGitFrontier(ctx, root, baseSHA, commit)
			if frontierErr != nil {
				result.Unavailable["git_frontier"] = "git_frontier_unavailable"
			} else {
				result.GitFrontier = stringPointer(frontier)
			}
		}
	}

	db, dbErr := store.OpenReadOnlyExistingWithWAL(root, store.WorkspaceDBPath(root))
	if dbErr != nil {
		result.GenerationReason["docs"] = "docs_generation_unavailable"
		result.GenerationReason["catalog"] = "catalog_generation_unavailable"
		result.GenerationReason["graph"] = "graph_generation_unavailable"
	} else {
		defer db.Close()
		read := func(name, key string) {
			value, ok, readErr := store.WorkspaceMetaValue(ctx, db, key)
			if readErr != nil {
				result.GenerationReason[name] = name + "_generation_unavailable"
				return
			}
			if !ok || strings.TrimSpace(value) == "" {
				result.GenerationReason[name] = name + "_generation_missing"
				return
			}
			result.Generations[name] = value
		}
		read("docs", store.WorkspaceMetaActiveDocsGeneration)
		read("catalog", store.WorkspaceMetaActiveCatalogGeneration)
		graph, graphFound, graphErr := store.ActiveGraphGeneration(ctx, db)
		if graphErr != nil {
			result.GenerationReason["graph"] = "graph_generation_unavailable"
		} else if !graphFound || graph == (model.GraphDigest{}) {
			result.GenerationReason["graph"] = "graph_generation_missing"
		} else {
			result.Generations["graph"] = graph.String()
			freshness, freshnessErr := store.GraphFreshness(ctx, db, graph.String())
			switch {
			case freshnessErr != nil:
				result.GraphFreshness = "unavailable"
				result.GenerationReason["graph_freshness"] = "graph_freshness_unavailable"
			case freshness.State == model.GraphFreshnessCurrent:
				result.GraphFreshness = "current"
			case freshness.State == model.GraphFreshnessStale || freshness.State == model.GraphFreshnessLagging:
				result.GraphFreshness = "stale"
			case freshness.State == model.GraphFreshnessInvalid:
				result.GraphFreshness = "unavailable"
				incompatibleGraph = true
				result.GenerationReason["graph_freshness"] = "graph_generation_incompatible"
			default:
				result.GraphFreshness = "unavailable"
				result.GenerationReason["graph_freshness"] = "graph_freshness_unavailable"
			}
		}
	}
	_, docsOK := result.Generations["docs"].(string)
	_, catalogOK := result.Generations["catalog"].(string)
	_, graphOK := result.Generations["graph"].(string)
	if incompatibleGraph {
		result.Coherence = "incompatible"
	} else if docsOK && catalogOK {
		if graphOK && result.GraphFreshness == "current" {
			result.Coherence = "coherent"
		} else {
			result.Coherence = "degraded"
		}
	}
	if len(result.GenerationReason) == 0 {
		result.GenerationReason = nil
	}
	if len(result.Unavailable) == 0 {
		result.Unavailable = nil
	}
	return result
}

func calculateGitFrontier(ctx context.Context, root, base, head string) (string, error) {
	mergeBase, err := gitOutput(ctx, root, "merge-base", base, head)
	if err != nil {
		return "", err
	}
	committed, err := gitOutputBytes(ctx, root, "diff", "--binary", "--no-ext-diff", mergeBase+"..."+head, "--")
	if err != nil {
		return "", err
	}
	working, err := gitOutputBytes(ctx, root, "diff", "--binary", "--no-ext-diff", "HEAD", "--")
	if err != nil {
		return "", err
	}
	untracked, err := gitOutputBytes(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", err
	}
	paths := make([]string, 0)
	for _, path := range strings.Split(string(untracked), "\x00") {
		if path != "" {
			paths = append(paths, filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))))
		}
	}
	sort.Strings(paths)
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "mi-lsp-git-frontier-v1\x00base=%s\x00merge-base=%s\x00head=%s\x00", base, mergeBase, head)
	writeDigestPart(hash, "committed", committed)
	writeDigestPart(hash, "working", working)
	for _, path := range paths {
		data, readErr := readFrontierPath(root, path)
		if readErr != nil {
			return "", readErr
		}
		writeDigestPart(hash, path, data)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readFrontierPath(root, relative string) ([]byte, error) {
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}
		return []byte("symlink:" + filepath.ToSlash(target)), nil
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("untracked path is not a regular file: %s", relative)
	}
	return os.ReadFile(path)
}

type digestWriter interface{ Write([]byte) (int, error) }

func writeDigestPart(w digestWriter, name string, data []byte) {
	_, _ = fmt.Fprintf(w, "%d:%s:%d:", len(name), name, len(data))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte{0})
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	data, err := gitOutputBytes(ctx, root, args...)
	return strings.TrimSpace(string(data)), err
}

func gitOutputBytes(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	data, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return data, nil
}

func stringPointer(value string) *string { return &value }
