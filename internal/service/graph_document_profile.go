package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// graphDocumentProfileInput binds the canonical profile choice to the bytes
// actually decoded. External is a canonical configuration input, not permission
// to enumerate external documents or to mask an external graph owner. Callers
// must include Fingerprint in their cache key and recheck it before exposing a
// cached or newly assembled overlay. This is not an atomic filesystem snapshot.
type graphDocumentProfileInput struct {
	Profile     model.DocsReadProfile
	Path        string
	Source      string
	External    bool
	Missing     bool
	Warnings    []string
	Fingerprint string
}

// captureWorktreeGraphDocumentProfile follows canonical profile discovery and
// default/fallback semantics without making LoadProfile reread a file. A missing
// default profile is a real default input, not a failed project-profile read.
// Discovery may select a profile in a resolved external canon. Only that exact
// selected configuration file is read, using the same checked-byte reader.
func captureWorktreeGraphDocumentProfile(ctx context.Context, root string) (graphDocumentProfileInput, error) {
	var input graphDocumentProfileInput
	if err := ctx.Err(); err != nil {
		return input, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return input, err
	}
	selected, err := filepath.Abs(docgraph.DiscoverProfilePath(root))
	if err != nil {
		return input, err
	}
	relative, err := filepath.Rel(root, selected)
	if err != nil {
		return input, err
	}
	input.Path = selected
	input.External = filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
	readRoot, readPath := root, filepath.ToSlash(relative)
	if input.External {
		readRoot, readPath = filepath.Dir(selected), filepath.Base(selected)
	}
	content, missing, err := readWorktreeGraphDocument(ctx, readRoot, readPath)
	if err != nil {
		return input, err
	}
	input.Missing = missing
	input.Source = "default"
	if missing {
		// Discovery returns an absent default only after finding no selected
		// project/canon profile. Losing a previously selected external file is
		// indeterminate, not authorization to switch to a default mid-query.
		if input.External || selected != filepath.Clean(docgraph.ProfilePath(root)) {
			return input, overlayGraphUnavailable("selected document profile disappeared")
		}
		input.Profile, input.Warnings, err = decodeWorktreeGraphDocumentProfile(nil)
	} else {
		input.Profile, input.Warnings, err = decodeWorktreeGraphDocumentProfile(content)
		if err != nil {
			decodeErr := err
			input.Profile, input.Warnings, err = decodeWorktreeGraphDocumentProfile(nil)
			input.Warnings = append(input.Warnings, fmt.Sprintf("read-model parse failed; using defaults: %v", decodeErr))
		} else {
			input.Source = "project"
		}
	}
	if err != nil {
		return input, err
	}
	// Detect a changed selection as well as changed bytes. A new higher
	// priority profile can otherwise leave the old selected file untouched.
	if filepath.Clean(docgraph.DiscoverProfilePath(root)) != selected {
		return input, overlayGraphUnavailable("document profile selection changed during capture")
	}
	contentDigest := sha256.Sum256(content)
	identity, err := json.Marshal(struct {
		Root, Path, Source string
		External, Missing  bool
		ContentDigest      [sha256.Size]byte
		Warnings           []string
	}{root, selected, input.Source, input.External, missing, contentDigest, input.Warnings})
	if err != nil {
		return input, err
	}
	digest := sha256.Sum256(append([]byte("mi-lsp:worktree-doc-profile-input:v1\x00"), identity...))
	input.Fingerprint = hex.EncodeToString(digest[:])
	return input, nil
}

func (input graphDocumentProfileInput) recheck(ctx context.Context, root string) error {
	current, err := captureWorktreeGraphDocumentProfile(ctx, root)
	if err != nil {
		return err
	}
	if input.Fingerprint == "" || current.Fingerprint != input.Fingerprint {
		return overlayGraphUnavailable("document profile input changed during graph observation")
	}
	return nil
}

const maxWorktreeGraphDocumentPaths = 4096

type graphDocumentDeltaPlan struct {
	ProfileChanged  bool
	ProfileControls []string
	AffectedPaths   []string
	DomainPaths     []string
}

func decodeWorktreeGraphDocumentProfile(content []byte) (model.DocsReadProfile, []string, error) {
	profile := docgraph.DefaultProfile()
	if len(content) > 0 {
		if _, err := toml.Decode(string(content), &profile); err != nil {
			return model.DocsReadProfile{}, nil, err
		}
		if profile.Version == 0 {
			profile.Version = 1
		}
	}
	var warnings []string
	if profile.WikiMap != nil {
		roots, rootWarnings := model.SafeRoots(profile.WikiMap.Roots)
		profile.WikiMap.Roots = roots
		warnings = append(warnings, rootWarnings...)
		seenHubIDs := map[string]struct{}{}
		hubs := make([]model.WikiMapHubConfig, 0, len(profile.WikiMap.Hubs))
		for i, hub := range profile.WikiMap.Hubs {
			hub.ID = strings.TrimSpace(hub.ID)
			hub.Title = strings.TrimSpace(hub.Title)
			patterns, patternWarnings := model.SafePatterns(hub.Patterns)
			warnings = append(warnings, patternWarnings...)
			_, duplicate := seenHubIDs[hub.ID]
			if hub.ID == "" || hub.Title == "" || len(patterns) == 0 || duplicate {
				warnings = append(warnings, fmt.Sprintf("wiki_map_hub_rejected_%d", i+1))
				continue
			}
			hub.Patterns = patterns
			seenHubIDs[hub.ID] = struct{}{}
			hubs = append(hubs, hub)
		}
		profile.WikiMap.Hubs = hubs
	}
	profile.GenericDocs.Paths = model.AppendWikiMapRoots(profile.GenericDocs.Paths, &profile)
	return profile, warnings, nil
}

func planWorktreeGraphDocumentDelta(ctx context.Context, base *store.GraphQuerySnapshot, root, baseRoot string, project model.ProjectFile, overlay workspace.WorktreeOverlay, input graphDocumentProfileInput) (graphDocumentDeltaPlan, error) {
	var plan graphDocumentDeltaPlan
	if input.Fingerprint == "" || len(input.Warnings) > 0 {
		return plan, overlayGraphUnavailable("document profile input is not exact")
	}
	pinned, err := base.WorktreeDocumentProfileContext(ctx)
	if err != nil {
		return plan, err
	}
	if pinned.DocumentCount > 0 && !pinned.Compatible {
		return plan, overlayGraphUnavailable("pinned document profile witness is incompatible")
	}
	currentID := graphProfilePathIdentity(root, input.Path)
	basePath := docgraph.DiscoverProfilePath(baseRoot)
	baseID := graphProfilePathIdentity(baseRoot, basePath)
	currentDigest := store.AuthorityConfigDigestForProfile(input.Profile, nil)
	plan.ProfileChanged = currentID != baseID || (input.External && pinned.DocumentCount == 0)
	if pinned.DocumentCount > 0 && pinned.Compatible {
		plan.ProfileChanged = plan.ProfileChanged || currentDigest != pinned.AuthorityDigest
	}
	changed := graphDeltaPathUnion(overlay.Changed, overlay.Deleted)
	changedSet := make(map[string]bool, len(changed))
	for _, path := range changed {
		changedSet[path] = true
		name := filepath.Base(path)
		if name == ".gitignore" || name == ".milspignore" || name == "project.toml" {
			return plan, overlayGraphUnavailable("worktree document inventory configuration changed without a certified owner plan")
		}
	}
	controls := map[string]bool{}
	for _, candidate := range []struct{ root, path string }{
		{root, input.Path},
		{baseRoot, basePath},
	} {
		if rel, ok := graphProfileWorkspacePath(candidate.root, candidate.path); ok {
			controls[rel] = true
			if changedSet[rel] {
				plan.ProfileChanged = true
			}
		}
	}
	for path := range changedSet {
		if controls[path] {
			plan.ProfileControls = append(plan.ProfileControls, path)
		}
	}
	sort.Strings(plan.ProfileControls)

	baseOwners, err := base.WorktreeDocumentOwnerPaths(ctx)
	if err != nil {
		return plan, err
	}
	plan.DomainPaths = append(plan.DomainPaths, baseOwners...)
	hasMarkdownChanges := false
	for _, path := range changed {
		if strings.EqualFold(filepath.Ext(path), ".md") && !controls[path] {
			hasMarkdownChanges = true
		}
	}
	currentDocs := []string{}
	if plan.ProfileChanged || hasMarkdownChanges {
		currentDocs, err = indexer.WorktreeGraphDocumentPaths(ctx, root, input.Profile, project.Ignore.ExtraPatterns, maxWorktreeGraphDocumentPaths)
		if err != nil {
			return plan, err
		}
		plan.DomainPaths = append(plan.DomainPaths, currentDocs...)
	}
	if plan.ProfileChanged {
		plan.AffectedPaths = append(plan.AffectedPaths, baseOwners...)
		plan.AffectedPaths = append(plan.AffectedPaths, currentDocs...)
	} else if hasMarkdownChanges {
		currentSet, priorSet := map[string]bool{}, map[string]bool{}
		for _, path := range currentDocs {
			currentSet[path] = true
		}
		for _, path := range baseOwners {
			priorSet[path] = true
		}
		for _, path := range changed {
			if !strings.EqualFold(filepath.Ext(path), ".md") || controls[path] {
				continue
			}
			if currentSet[path] || priorSet[path] {
				plan.AffectedPaths = append(plan.AffectedPaths, path)
			}
		}
	}
	plan.AffectedPaths = graphDeltaPathUnion(plan.AffectedPaths)
	plan.DomainPaths = graphDeltaPathUnion(plan.DomainPaths)
	if len(plan.AffectedPaths) > maxWorktreeGraphDocumentPaths {
		return plan, overlayGraphUnavailable("changed worktree document selectors exceed bound")
	}
	if len(plan.DomainPaths) > maxWorktreeGraphDocumentPaths {
		return plan, overlayGraphUnavailable("worktree document domain exceeds bound")
	}
	return plan, nil
}

func graphProfileWorkspacePath(root, path string) (string, bool) {
	rootAbs, rootErr := filepath.Abs(root)
	pathAbs, pathErr := filepath.Abs(path)
	if rootErr != nil || pathErr != nil {
		return "", false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func graphProfilePathIdentity(root, path string) string {
	if rel, ok := graphProfileWorkspacePath(root, path); ok {
		return "workspace:" + rel
	}
	return "external:" + filepath.Clean(path)
}
