package docgraph

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/fgpaz/mi-lsp/internal/docidentity"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

var (
	docIDPattern            = regexp.MustCompile(`\b(?:FL|RS|RF|TP|TECH|CT|DB|AE)-[A-Z0-9-]+\b`)
	markdownLinkPattern     = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	wikiLinkPattern         = regexp.MustCompile(`!?\[\[([^\]]+?)\]\]`)
	inlineCodePattern       = regexp.MustCompile("`([^`]+)`")
	pascalSymbolPattern     = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]+\b`)
	semanticRelationPattern = regexp.MustCompile(`(?i)^\s*[-*+]\s+(related|depends|supports|contradicts|supersedes)\s*:\s*(.*?)\s*$`)
)

func isSnapshotPath(path string) bool {
	lower := strings.ToLower(path)
	snapshots := []string{"/old/", "/archive/", "/deprecated/", "/historico/", "/legacy/"}
	for _, seg := range snapshots {
		if strings.Contains(lower, seg) {
			return true
		}
		// Also check for segment at start of path (e.g., "old/foo.md")
		if len(seg) > 1 && strings.HasPrefix(lower, seg[1:]) {
			return true
		}
	}
	return false
}

type RFFrontMatter struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Implements []string `yaml:"implements"`
	Tests      []string `yaml:"tests"`
}

type Progress struct {
	Stage      string
	Path       string
	Docs       int
	FilesTotal int
	// Parsed/Skipped report docs.read path-level skip-reparse stats when available.
	Parsed  int
	Skipped int
	Force   bool
}

type ProgressFunc func(context.Context, Progress) error

// PriorDocSnapshot holds a previous docs index keyed by relative path so
// IndexWorkspaceDocs can skip markdown/wiki-source reparse when content_hash matches.
type PriorDocSnapshot struct {
	Docs     map[string]model.DocRecord
	Edges    map[string][]model.DocEdge
	Mentions map[string][]model.DocMention
	Blocks   map[string][]model.DocSourceBlock
	Records  map[string][]model.DocSourceRecord
	Bindings map[string][]model.DocArtifactBinding
}

// BuildPriorDocSnapshot indexes prior docs facts by path for skip-reparse reuse.
func BuildPriorDocSnapshot(
	docs []model.DocRecord,
	edges []model.DocEdge,
	mentions []model.DocMention,
	blocks []model.DocSourceBlock,
	records []model.DocSourceRecord,
	bindings []model.DocArtifactBinding,
) *PriorDocSnapshot {
	if len(docs) == 0 {
		return nil
	}
	prior := &PriorDocSnapshot{
		Docs:     make(map[string]model.DocRecord, len(docs)),
		Edges:    make(map[string][]model.DocEdge),
		Mentions: make(map[string][]model.DocMention),
		Blocks:   make(map[string][]model.DocSourceBlock),
		Records:  make(map[string][]model.DocSourceRecord),
		Bindings: make(map[string][]model.DocArtifactBinding),
	}
	for _, doc := range docs {
		if doc.Path == "" || doc.ContentHash == "" {
			continue
		}
		prior.Docs[doc.Path] = doc
	}
	if len(prior.Docs) == 0 {
		return nil
	}
	for _, edge := range edges {
		if edge.FromPath == "" {
			continue
		}
		prior.Edges[edge.FromPath] = append(prior.Edges[edge.FromPath], edge)
	}
	for _, mention := range mentions {
		if mention.DocPath == "" {
			continue
		}
		prior.Mentions[mention.DocPath] = append(prior.Mentions[mention.DocPath], mention)
	}
	for _, block := range blocks {
		if block.DocPath == "" {
			continue
		}
		prior.Blocks[block.DocPath] = append(prior.Blocks[block.DocPath], block)
	}
	for _, record := range records {
		if record.DocPath == "" {
			continue
		}
		prior.Records[record.DocPath] = append(prior.Records[record.DocPath], record)
	}
	for _, binding := range bindings {
		if binding.DocPath == "" {
			continue
		}
		prior.Bindings[binding.DocPath] = append(prior.Bindings[binding.DocPath], binding)
	}
	return prior
}

func ProfilePath(root string) string {
	return filepath.Join(root, filepath.FromSlash(defaultProjectionRelPath))
}

func DiscoverProfilePath(root string) string {
	defaultPath := ProfilePath(root)
	if pathExists(defaultPath) {
		return defaultPath
	}
	canons := tryResolvedCanons(root)
	var producto, others []workspace.ResolvedCanon
	for _, canon := range canons {
		if strings.EqualFold(canon.Role, "producto") {
			producto = append(producto, canon)
		} else {
			others = append(others, canon)
		}
	}
	for _, group := range [][]workspace.ResolvedCanon{producto, others} {
		for _, canon := range group {
			candidate := filepath.Join(canon.AbsRoot, "_mi-lsp", "read-model.toml")
			if pathExists(candidate) {
				return candidate
			}
		}
	}
	ingenieria := filepath.Join(root, "Ingenieria", "_mi-lsp", "read-model.toml")
	if pathExists(ingenieria) {
		return ingenieria
	}
	return defaultPath
}

func DefaultProfile() model.DocsReadProfile {
	return model.DocsReadProfile{
		Version: 1,
		Families: []model.DocsReadFamily{
			{
				Name:           "functional",
				IntentKeywords: []string{"scope", "outcome", "result", "resultado", "solucion", "solution", "flow", "feature", "behavior", "rs", "rf", "fl", "test", "workflow", "journey"},
				Paths: []string{
					".docs/wiki/00_governance.md",
					".docs/wiki/00_retirements.md",
					".docs/wiki/01_scope.md",
					".docs/wiki/02_outcomes/RS-*.md",
					".docs/wiki/03_architecture.md",
					".docs/wiki/03_architecture/ADR-*.md",
					".docs/wiki/04_flows/FL-*.md",
					".docs/wiki/05_requirements/RF-*.md",
					".docs/wiki/05_requirements/data-model.md",
					".docs/wiki/06_tests/TP-*.md",
				},
			},
			{
				Name:           "technical",
				IntentKeywords: []string{"technical", "daemon", "worker", "runtime", "backend", "contract", "protocol", "search", "context", "refs", "service", "index", "routing", "ae", "agent engineering", "release", "distribution", "binary", "install"},
				Paths: []string{
					".docs/wiki/07_technical.md",
					".docs/wiki/07_technical/TECH-*.md",
					".docs/wiki/07_technical/db/DB-*.md",
					".docs/wiki/07_technical/contracts/CT-*.md",
				},
			},
			{
				Name:           "ux",
				IntentKeywords: []string{"ux", "ui", "frontend", "visual", "design", "journey", "experience", "pattern", "interface"},
				Paths: []string{
					".docs/wiki/10_*.md",
					".docs/wiki/11_*.md",
					".docs/wiki/12_*.md",
					".docs/wiki/13_*.md",
					".docs/wiki/14_*.md",
					".docs/wiki/15_*.md",
					".docs/wiki/16_*.md",
					".docs/wiki/17_*.md",
					".docs/wiki/18_*.md",
					".docs/wiki/19_*.md",
					".docs/wiki/20_*.md",
					".docs/wiki/21_*.md",
					".docs/wiki/22_*.md",
					".docs/wiki/23_uxui/",
				},
			},
		},
		GenericDocs: model.DocsGenericFallback{
			Paths: []string{"README.md", "README*.md", "docs/", ".docs/"},
		},
		ReadingPack: model.DocsReadingPackProfile{
			MaxDocs:              6,
			FunctionalStageOrder: []string{"governance", "scope", "outcome", "architecture", "flow", "requirements", "data", "tests"},
			TechnicalStageOrder:  []string{"governance", "scope", "architecture", "technical_baseline", "technical_detail", "physical_data", "contracts"},
			UXStageOrder:         []string{"governance", "scope", "architecture", "ux_global", "ux_research", "ux_spec", "ux_handoff"},
		},
	}
}

func LoadProfile(root string) (model.DocsReadProfile, string, []string) {
	profile := DefaultProfile()
	path := DiscoverProfilePath(root)
	source := "default"
	warnings := []string{}
	if _, statErr := os.Stat(path); statErr == nil {
		source = "project"
		if _, decodeErr := toml.DecodeFile(path, &profile); decodeErr != nil {
			profile = DefaultProfile()
			source = "default"
			warnings = append(warnings, fmt.Sprintf("read-model parse failed; using defaults: %v", decodeErr))
		} else if profile.Version == 0 {
			profile.Version = 1
		}
	}
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
	return profile, source, warnings
}

func IndexWorkspaceDocs(ctx context.Context, root string, matcher *workspace.IgnoreMatcher) ([]model.DocRecord, []model.DocEdge, []model.DocMention, []string, error) {
	docs, edges, mentions, _, _, warnings, err := IndexWorkspaceDocsWithSourcesWithProgress(ctx, root, matcher, nil)
	return docs, edges, mentions, warnings, err
}

func IndexWorkspaceDocsWithProgress(ctx context.Context, root string, matcher *workspace.IgnoreMatcher, progress ProgressFunc) ([]model.DocRecord, []model.DocEdge, []model.DocMention, []string, error) {
	docs, edges, mentions, _, _, warnings, err := IndexWorkspaceDocsWithSourcesWithProgress(ctx, root, matcher, progress)
	return docs, edges, mentions, warnings, err
}

func IndexWorkspaceDocsWithSourcesWithProgress(ctx context.Context, root string, matcher *workspace.IgnoreMatcher, progress ProgressFunc) ([]model.DocRecord, []model.DocEdge, []model.DocMention, []model.DocSourceBlock, []model.DocSourceRecord, []string, error) {
	return IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx, root, matcher, progress, nil)
}

// IndexWorkspaceDocsWithSourcesWithProgressPrior indexes docs and skips markdown/wiki-source
// reparse when prior content_hash still matches the on-disk file bytes.
func IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx context.Context, root string, matcher *workspace.IgnoreMatcher, progress ProgressFunc, prior *PriorDocSnapshot) ([]model.DocRecord, []model.DocEdge, []model.DocMention, []model.DocSourceBlock, []model.DocSourceRecord, []string, error) {
	docs, edges, mentions, sourceBlocks, sourceRecords, _, warnings, err := IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings(ctx, root, matcher, progress, prior)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	return docs, edges, mentions, sourceBlocks, sourceRecords, warnings, nil
}

// IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings indexes docs and returns bindings.
func IndexWorkspaceDocsWithSourcesWithProgressPriorWithBindings(ctx context.Context, root string, matcher *workspace.IgnoreMatcher, progress ProgressFunc, prior *PriorDocSnapshot) ([]model.DocRecord, []model.DocEdge, []model.DocMention, []model.DocSourceBlock, []model.DocSourceRecord, []model.DocArtifactBinding, []string, error) {
	collectStarted := time.Now()
	profile, _, warnings := LoadProfile(root)
	if err := reportProgress(ctx, progress, Progress{Stage: "docs.collect", Force: true}); err != nil {
		return nil, nil, nil, nil, nil, nil, warnings, err
	}
	candidates, err := collectDocCandidates(ctx, root, profile, matcher)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, warnings, err
	}
	kernelCandidates, kernelWarnings := collectKernelV2CanonDocCandidates(root, profile)
	warnings = append(warnings, kernelWarnings...)
	kernelCandidates = excludeExistingDocSources(candidates, kernelCandidates)
	candidates = mergeDocCandidates(candidates, kernelCandidates)
	canonCandidates, canonWarnings, canonErr := collectCanonDocCandidates(ctx, root, profile)
	warnings = append(warnings, canonWarnings...)
	if canonErr != nil {
		return nil, nil, nil, nil, nil, nil, warnings, canonErr
	}
	candidates = mergeDocCandidates(candidates, canonCandidates)
	setupCandidates, setupWarnings, setupErr := collectSetupWikiDocCandidates(ctx, root, profile)
	warnings = append(warnings, setupWarnings...)
	if setupErr != nil {
		return nil, nil, nil, nil, nil, nil, warnings, setupErr
	}
	setupCandidates = excludeExistingDocSources(candidates, setupCandidates)
	candidates = mergeDocCandidates(candidates, setupCandidates)
	if err := reportProgress(ctx, progress, Progress{
		Stage:      "docs.collect",
		Path:       fmt.Sprintf("elapsed_ms=%d", time.Since(collectStarted).Milliseconds()),
		FilesTotal: len(candidates),
		Force:      true,
	}); err != nil {
		return nil, nil, nil, nil, nil, nil, warnings, err
	}

	readStarted := time.Now()
	if err := reportProgress(ctx, progress, Progress{Stage: "docs.read", FilesTotal: len(candidates), Force: true}); err != nil {
		return nil, nil, nil, nil, nil, nil, warnings, err
	}

	type docWorkResult struct {
		doc           model.DocRecord
		mentions      []model.DocMention
		edges         []model.DocEdge
		sourceBlocks  []model.DocSourceBlock
		sourceRecords []model.DocSourceRecord
		bindings      []model.DocArtifactBinding
		skipped       bool
		warning       string
		err           error
	}

	results := make([]docWorkResult, len(candidates))
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		workers = 2
	}
	if workers > 8 {
		workers = 8
	}
	if len(candidates) < workers {
		workers = len(candidates)
	}
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan int, len(candidates))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := ctx.Err(); err != nil {
					results[i] = docWorkResult{err: err}
					continue
				}
				candidate := candidates[i]
				content, err := os.ReadFile(candidate.path)
				if err != nil {
					results[i] = docWorkResult{warning: fmt.Sprintf("doc read failed for %s: %v", candidate.relativePath, err)}
					continue
				}
				contentHash := digest(content)
				if prior != nil {
					if prev, ok := prior.Docs[candidate.relativePath]; ok && prev.ContentHash == contentHash {
						currentID, identityDeclared := wikisource.DocumentIdentity(string(content))
						if !identityDeclared {
							currentID = legacyDocumentID(candidate.relativePath, extractTitle(content))
						}
						wikiLinks := wikisource.WikiContractLinks(string(content))
						if currentID == prev.DocID && wikiContractLinksCached(candidate.relativePath, wikiLinks, prior.Edges[candidate.relativePath], prior.Docs, profile.EffectiveWikiMapRoots()) {
							doc := prev
							doc.Path = candidate.relativePath
							doc.Layer = candidate.layer
							doc.Family = candidate.family
							doc.ContentHash = contentHash
							doc.IsSnapshot = isSnapshotPath(candidate.relativePath)
							// Preserve title/snippet/search_text/doc_id/IndexedAt from prior; content is identical.
							results[i] = docWorkResult{
								doc:           doc,
								mentions:      append([]model.DocMention(nil), prior.Mentions[candidate.relativePath]...),
								edges:         append([]model.DocEdge(nil), prior.Edges[candidate.relativePath]...),
								sourceBlocks:  append([]model.DocSourceBlock(nil), prior.Blocks[candidate.relativePath]...),
								sourceRecords: append([]model.DocSourceRecord(nil), prior.Records[candidate.relativePath]...),
								bindings:      append([]model.DocArtifactBinding(nil), prior.Bindings[candidate.relativePath]...),
								skipped:       true,
							}
							continue
						}
						// The bytes are unchanged, but the ownership rules changed;
						// reparse so an old body-derived identity cannot persist.
					}
				}
				doc, docEdges, mentions, sourceBlocks, sourceRecords, bindings, parseErr := parseDocContent(root, candidate.relativePath, content, profile)
				if parseErr != nil {
					results[i] = docWorkResult{warning: fmt.Sprintf("doc parse failed for %s: %v", candidate.relativePath, parseErr)}
					continue
				}
				results[i] = docWorkResult{
					doc:           doc,
					mentions:      mentions,
					edges:         docEdges,
					sourceBlocks:  sourceBlocks,
					sourceRecords: sourceRecords,
					bindings:      bindings,
				}
			}
		}()
	}
	for i := range candidates {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, nil, nil, nil, warnings, err
	}

	docs := make([]model.DocRecord, 0, len(candidates))
	edges := make([]model.DocEdge, 0)
	mentions := make([]model.DocMention, 0)
	sourceBlocks := make([]model.DocSourceBlock, 0)
	sourceRecords := make([]model.DocSourceRecord, 0)
	bindings := make([]model.DocArtifactBinding, 0)
	parsed := 0
	skipped := 0

	for _, result := range results {
		if result.err != nil {
			return nil, nil, nil, nil, nil, nil, warnings, result.err
		}
		if result.warning != "" {
			warnings = append(warnings, result.warning)
			continue
		}
		if result.doc.Path == "" {
			continue
		}
		if result.skipped {
			skipped++
		} else {
			parsed++
		}
		docs = append(docs, result.doc)
		mentions = append(mentions, result.mentions...)
		sourceBlocks = append(sourceBlocks, result.sourceBlocks...)
		sourceRecords = append(sourceRecords, result.sourceRecords...)
		bindings = append(bindings, result.bindings...)
		edges = append(edges, result.edges...)
	}

	if err := reportProgress(ctx, progress, Progress{
		Stage:      "docs.read",
		Path:       fmt.Sprintf("elapsed_ms=%d parsed=%d skipped=%d", time.Since(readStarted).Milliseconds(), parsed, skipped),
		Docs:       len(docs),
		FilesTotal: len(candidates),
		Parsed:     parsed,
		Skipped:    skipped,
		Force:      true,
	}); err != nil {
		return nil, nil, nil, nil, nil, nil, warnings, err
	}
	// Keep the existing public summary even when no prior rows were reused.
	// Both counters come from successful path results, not from the clean flag.
	warnings = append(warnings, fmt.Sprintf("docs_skip_reparse parsed=%d skipped=%d", parsed, skipped))

	// Missing from today's candidate scan is not proof of deletion. Retain
	// alive-but-excluded/read-failed owners and terminal retired history.
	if prior != nil {
		included := make(map[string]bool, len(docs))
		for _, doc := range docs {
			included[doc.Path] = true
		}
		for path, doc := range prior.Docs {
			if included[path] {
				continue
			}
			retired := doc.BindingSources != nil && doc.BindingSources.DocumentLifecycle == model.DocLifecycleRetired
			for _, binding := range prior.Bindings[path] {
				retired = retired || binding.DocLifecycle == model.DocLifecycleRetired
			}
			_, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(path)))
			if !retired && os.IsNotExist(statErr) {
				continue
			}
			docs = append(docs, doc)
			edges = append(edges, prior.Edges[path]...)
			mentions = append(mentions, prior.Mentions[path]...)
			sourceBlocks = append(sourceBlocks, prior.Blocks[path]...)
			sourceRecords = append(sourceRecords, prior.Records[path]...)
			bindings = append(bindings, prior.Bindings[path]...)
			warnings = append(warnings, "docs_owner_retained:"+path)
		}
	}

	edges = resolveDocEdges(docs, edges, profile.EffectiveWikiMapRoots())
	edges = appendStructuralDocEdges(docs, edges)

	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Family == docs[j].Family {
			if docs[i].Layer == docs[j].Layer {
				return docs[i].Path < docs[j].Path
			}
			return docs[i].Layer < docs[j].Layer
		}
		return docs[i].Family < docs[j].Family
	})
	return docs, edges, mentions, sourceBlocks, sourceRecords, bindings, warnings, nil
}

func wikiContractLinksCached(docPath string, links []wikisource.WikiLink, priorEdges []model.DocEdge, priorDocs map[string]model.DocRecord, roots []string) bool {
	if len(links) == 0 {
		return true
	}
	docs := make([]model.DocRecord, 0, len(priorDocs))
	for _, doc := range priorDocs {
		docs = append(docs, doc)
	}
	pathIndex := newDocResolutionIndex(docs)
	for _, link := range links {
		found := false
		for _, edge := range priorEdges {
			if edge.Kind != link.Relation || strings.TrimSpace(edge.Label) != strings.TrimSpace(link.Symbol) || (link.Relation == "code" && edge.UnresolvedReason != "") {
				continue
			}
			if link.DocID != "" {
				if edge.ToDocID == link.DocID {
					found = true
					break
				}
				if doc, ok := priorDocs[edge.ToPath]; ok && doc.DocID == link.DocID {
					found = true
					break
				}
			}
			if link.Path != "" {
				target := normalizeDocCandidate(edge.ToPath)
				if target == normalizeDocCandidate(link.Path) {
					found = true
					break
				}
				if link.Relation != "code" {
					for _, candidate := range pathIndex.resolvePath(docPath, link.Path, roots) {
						if target == candidate {
							found = true
							break
						}
					}
					if found {
						break
					}
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ParseSingleDoc parses a single markdown document and returns its complete
// doc artifact model. It shares the exact same parsing logic as the full-index
// parser, accepting the workspace root, repo-relative path, and pre-loaded
// profile. The caller is responsible for reading the file content and passing
// it; this function only does the parsing (title, doc_id, edges, mentions,
// blocks, records, bindings). Layer and family always come from
// profile/path classification.
//
// Returns (DocRecord, edges, mentions, sourceBlocks, sourceRecords, bindings,
// warning, error). A non-empty warning indicates a non-fatal parse issue.
func ParseSingleDoc(
	root string,
	relPath string,
	content []byte,
	profile model.DocsReadProfile,
) (model.DocRecord, []model.DocEdge, []model.DocMention, []model.DocSourceBlock, []model.DocSourceRecord, []model.DocArtifactBinding, string, error) {
	doc, edges, mentions, sourceBlocks, sourceRecords, bindings, err := parseDocContent(root, relPath, content, profile)
	return doc, edges, mentions, sourceBlocks, sourceRecords, bindings, "", err
}

// parseDocContent is the one document parser used by both full and incremental
// indexing. Classification is derived from the same profile/path precedence as
// collectDocCandidates; callers cannot accidentally publish empty layer or
// family values.
func parseDocContent(
	root string,
	relPath string,
	content []byte,
	profile model.DocsReadProfile,
) (model.DocRecord, []model.DocEdge, []model.DocMention, []model.DocSourceBlock, []model.DocSourceRecord, []model.DocArtifactBinding, error) {
	if strings.TrimSpace(relPath) == "" {
		return model.DocRecord{}, nil, nil, nil, nil, nil, fmt.Errorf("document path is empty")
	}
	family, layer := classifyDocPath(profile, relPath)
	contentHash := digest(content)
	title := extractTitle(content)
	docID, identityDeclared := wikisource.DocumentIdentity(string(content))
	if !identityDeclared {
		docID = legacyDocumentID(relPath, title)
	}
	now := time.Now().Unix()
	doc := model.DocRecord{
		Path:        relPath,
		Title:       title,
		DocID:       docID,
		Layer:       layer,
		Family:      family,
		Snippet:     extractSnippet(content),
		SearchText:  normalizeSearchText(title + "\n" + relPath + "\n" + string(content)),
		ContentHash: contentHash,
		IndexedAt:   now,
		IsSnapshot:  isSnapshotPath(relPath),
	}

	wikiContract := wikisource.ParseContractMetadata(string(content)).WikiContract == wikisource.WikiContractV1
	referenceContent := string(content)
	if wikiContract {
		// WIKI-CONTRACT links are already projected below. Mask their YAML
		// envelope before legacy reference extraction so nested links.code values
		// cannot be reinterpreted as implements/file mentions.
		referenceContent = maskWikiContractFrontMatter(referenceContent)
	}
	docMentions, docEdges := extractReferences(root, relPath, referenceContent)
	sourceDoc := wikisource.Parse(relPath, string(content), now)
	for _, link := range sourceDoc.WikiLinks {
		docEdges = append(docEdges, model.DocEdge{FromPath: relPath, ToPath: link.Path, ToDocID: link.DocID, Kind: link.Relation, Label: link.Symbol})
	}
	if strings.TrimSpace(sourceDoc.DocID) != "" && !identityDeclared {
		doc.DocID = sourceDoc.DocID
	}
	if !sourceDoc.DeclaresSource {
		metadata := wikisource.MetadataReferences(string(content))
		if len(metadata) > 0 {
			metadataMentions, metadataEdges := extractReferences(root, relPath, strings.Join(metadata, "\n"))
			docMentions = append(docMentions, metadataMentions...)
			docEdges = append(docEdges, metadataEdges...)
		}
	}

	mentions := append(docMentions, sourceDoc.Mentions...)
	collection := wikisource.SourceBindingDeclarations(sourceDoc)
	doc.BindingSources = &collection
	// Only this collection-carrying producer discharges A's transport omission.
	// Parser-only callers still receive it; rejection diagnostics stay intact.
	for _, omission := range sourceDoc.BindingOmissions {
		if omission == "explicit_incomplete:canonical_source_collection_not_persisted" {
			continue
		}
		mentions = append(mentions, model.DocMention{DocPath: relPath, MentionType: "binding_omission", MentionValue: omission})
	}
	sourceBlocks := wikisource.SourceBlocks(sourceDoc, now)
	sourceRecords := wikisource.SourceRecords(sourceDoc, now)
	bindings := wikisource.SourceBindings(sourceDoc, now)
	if !wikiContract {
		if fm := extractFrontMatter(content); fm != nil {
			for _, impl := range fm.Implements {
				impl = strings.TrimSpace(impl)
				if impl != "" {
					mentions = append(mentions, model.DocMention{DocPath: relPath, MentionType: "implements", MentionValue: impl, SourceBlock: "frontmatter"})
				}
			}
			for _, test := range fm.Tests {
				test = strings.TrimSpace(test)
				if test != "" {
					mentions = append(mentions, model.DocMention{DocPath: relPath, MentionType: "test_file", MentionValue: test, SourceBlock: "frontmatter"})
				}
			}
		}
	}
	return doc, docEdges, mentions, sourceBlocks, sourceRecords, bindings, nil
}

func classifyDocPath(profile model.DocsReadProfile, relPath string) (string, string) {
	normalized := filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(relPath), "./"))
	for _, family := range profile.Families {
		for _, pattern := range family.Paths {
			if matchesDocProfilePath(normalized, pattern) {
				name := strings.TrimSpace(family.Name)
				if name == "" {
					name = "generic"
				}
				return name, nonEmptyDocLayer(DetectLayerForPath(profile, normalized))
			}
		}
	}
	return "generic", nonEmptyDocLayer(DetectLayerForPath(profile, normalized))
}

func nonEmptyDocLayer(layer string) string {
	if strings.TrimSpace(layer) == "" {
		return "generic"
	}
	return layer
}

func matchesDocProfilePath(relPath, pattern string) bool {
	relPath = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(relPath), "./"))
	pattern = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(pattern), "./"))
	if relPath == "" || pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/") {
		prefix := strings.TrimSuffix(pattern, "/")
		return relPath == prefix || strings.HasPrefix(relPath, prefix+"/")
	}
	if marker := strings.Index(pattern, "/**"); marker >= 0 {
		prefix := strings.TrimSuffix(pattern[:marker], "/")
		if relPath == prefix || strings.HasPrefix(relPath, prefix+"/") {
			return true
		}
	}
	if strings.ContainsAny(pattern, "*?[") {
		matched, err := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(relPath))
		return err == nil && matched
	}
	return relPath == pattern
}

// ResolveDocEdges applies full-index path/doc-id resolution and structural
// edges against the supplied effective document set. Incremental callers use
// this after replacements/deletes have been folded into that set.
func ResolveDocEdges(docs []model.DocRecord, edges []model.DocEdge, profile model.DocsReadProfile) []model.DocEdge {
	resolved := resolveDocEdges(docs, edges, profile.EffectiveWikiMapRoots())
	return appendStructuralDocEdges(docs, resolved)
}

func reportProgress(ctx context.Context, progress ProgressFunc, value Progress) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if progress == nil {
		return nil
	}
	return progress(ctx, value)
}

type docCandidate struct {
	path         string
	relativePath string
	family       string
	layer        string
	priority     int
}

func collectDocCandidates(ctx context.Context, root string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher) ([]docCandidate, error) {
	seen := map[string]docCandidate{}
	addCandidate := func(absPath string, family string, priority int) {
		if strings.TrimSpace(family) == "" {
			family = "generic"
		}
		if matcher != nil && matcher.ShouldIgnore(root, absPath) {
			return
		}
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			return
		}
		rel := WorkspacePathForSource(root, absPath)
		if rel == "" {
			return
		}
		candidate := docCandidate{
			path:         absPath,
			relativePath: rel,
			family:       family,
			layer:        DetectLayerForPath(profile, rel),
			priority:     priority,
		}
		if existing, ok := seen[rel]; !ok || priority < existing.priority {
			seen[rel] = candidate
		}
	}

	for familyIdx, family := range profile.Families {
		for _, pattern := range family.Paths {
			if err := expandPattern(ctx, root, pattern, matcher, func(absPath string) {
				addCandidate(absPath, family.Name, familyIdx)
			}); err != nil {
				return nil, err
			}
		}
	}
	for _, pattern := range profile.GenericDocs.Paths {
		if err := expandPattern(ctx, root, pattern, matcher, func(absPath string) {
			addCandidate(absPath, "generic", len(profile.Families)+10)
		}); err != nil {
			return nil, err
		}
	}

	items := make([]docCandidate, 0, len(seen))
	for _, candidate := range seen {
		items = append(items, candidate)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].priority == items[j].priority {
			return items[i].relativePath < items[j].relativePath
		}
		return items[i].priority < items[j].priority
	})
	return items, nil
}

// collectKernelV2CanonDocCandidates adds only the five validated kernel modules
// for a caller that explicitly declares the kernel_v2 canon mode. The logical
// path is caller-scoped; path remains the guarded absolute host path used to read.
func collectKernelV2CanonDocCandidates(root string, profile model.DocsReadProfile) ([]docCandidate, []string) {
	config := profile.Governance.AECanon
	if !strings.EqualFold(strings.TrimSpace(config.Mode), "kernel_v2") {
		return nil, nil
	}
	status := inspectConfiguredAECanon(root, config, profile.Governance.Hierarchy, "read_model")
	if status.Status != "valid" {
		return nil, nil
	}
	kernelHome, err := resolveAEKernelHome()
	if err != nil {
		return nil, []string{"docs-only skipped kernel_v2 canon sources: kernel home is unavailable"}
	}
	items := make([]docCandidate, 0, len(requiredKernelV2CanonModules))
	for index, module := range requiredKernelV2CanonModules {
		absPath, err := safeRegularFileWithin(kernelHome, filepath.Join("canon", module))
		if err != nil {
			return nil, []string{"docs-only skipped kernel_v2 canon sources: a validated module could not be resolved safely"}
		}
		logicalPath := "ae_kernel_v2/canon/" + filepath.Base(module)
		items = append(items, docCandidate{
			path:         absPath,
			relativePath: logicalPath,
			family:       "generic",
			layer:        DetectLayerForPath(profile, logicalPath),
			priority:     len(profile.Families) + 20 + index,
		})
	}
	return items, nil
}

func collectCanonDocCandidates(ctx context.Context, root string, profile model.DocsReadProfile) ([]docCandidate, []string, error) {
	project, err := workspace.LoadProjectFile(root)
	if err != nil {
		return nil, []string{fmt.Sprintf("docs-only skipped declared [[canon]] roots: %v (fail-closed on those canon roots; local workspace docs were still indexed)", err)}, nil
	}
	if len(project.Canons) == 0 {
		return nil, nil, nil
	}
	wsRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, []string{fmt.Sprintf("docs-only skipped declared [[canon]] roots: %v (fail-closed on those canon roots; local workspace docs were still indexed)", err)}, nil
	}
	wsRoot = filepath.Clean(wsRoot)
	items := make([]docCandidate, 0)
	warnings := make([]string, 0)
	for _, canon := range project.Canons {
		if err := ctx.Err(); err != nil {
			return nil, warnings, err
		}
		one := model.ProjectFile{Canons: []model.WorkspaceCanon{canon}, CanonPolicy: project.CanonPolicy}
		resolved, resolveErr := workspace.ResolveCanons(wsRoot, one)
		if resolveErr != nil {
			warnings = append(warnings, fmt.Sprintf("docs-only skipped canon %q root %q: %v (fail-closed on that canon root; local workspace docs were still indexed)", strings.TrimSpace(canon.ID), strings.TrimSpace(canon.Root), resolveErr))
			continue
		}
		for _, item := range resolved {
			found, walkErr := walkCanonMarkdown(ctx, wsRoot, item, profile)
			if walkErr != nil {
				if err := ctx.Err(); err != nil {
					return nil, warnings, err
				}
				warnings = append(warnings, fmt.Sprintf("docs-only skipped canon %q root %q: %v (fail-closed on that canon root; local workspace docs were still indexed)", item.ID, item.DeclaredRoot, walkErr))
				continue
			}
			items = append(items, found...)
		}
	}
	return items, warnings, nil
}

func collectSetupWikiDocCandidates(ctx context.Context, workspaceRoot string, profile model.DocsReadProfile) ([]docCandidate, []string, error) {
	source := ResolveSetupWikiSource(workspaceRoot)
	if !source.Configured {
		return nil, nil, nil
	}
	if source.State != "resolved" || source.Root == "" {
		reason := source.ReasonCode
		if reason == "" {
			reason = "setup_wiki_source_unavailable"
		}
		return nil, []string{"setup_wiki_docs_skipped:" + reason}, nil
	}
	items := make([]docCandidate, 0)
	err := filepath.WalkDir(source.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".md", ".mdown", ".markdown":
		default:
			return nil
		}
		logicalPath := WorkspacePathForSource(workspaceRoot, path)
		items = append(items, docCandidate{
			path:         path,
			relativePath: logicalPath,
			family:       "generic",
			layer:        DetectLayerForPath(profile, logicalPath),
			priority:     len(profile.Families) + 30,
		})
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, []string{"setup_wiki_docs_skipped:setup_wiki_walk_failed"}, nil
	}
	return items, nil, nil
}

func walkCanonMarkdown(ctx context.Context, workspaceRoot string, canon workspace.ResolvedCanon, profile model.DocsReadProfile) ([]docCandidate, error) {
	items := make([]docCandidate, 0)
	err := filepath.WalkDir(canon.AbsRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(workspaceRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		items = append(items, docCandidate{
			path:         path,
			relativePath: rel,
			family:       "generic",
			layer:        DetectLayerForPath(profile, rel),
			priority:     100,
		})
		return nil
	})
	return items, err
}

func excludeExistingDocSources(existing, extra []docCandidate) []docCandidate {
	paths := make(map[string]struct{}, len(existing))
	for _, candidate := range existing {
		abs, err := filepath.Abs(candidate.path)
		if err == nil {
			paths[filepath.Clean(abs)] = struct{}{}
		}
	}
	filtered := extra[:0]
	for _, candidate := range extra {
		abs, err := filepath.Abs(candidate.path)
		if err != nil {
			continue
		}
		if _, exists := paths[filepath.Clean(abs)]; exists {
			continue
		}
		paths[filepath.Clean(abs)] = struct{}{}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func mergeDocCandidates(existing, extra []docCandidate) []docCandidate {
	if len(extra) == 0 {
		return existing
	}
	seen := make(map[string]docCandidate, len(existing)+len(extra))
	for _, candidate := range existing {
		seen[candidate.relativePath] = candidate
	}
	for _, candidate := range extra {
		if current, ok := seen[candidate.relativePath]; !ok || candidate.priority < current.priority {
			seen[candidate.relativePath] = candidate
		}
	}
	items := make([]docCandidate, 0, len(seen))
	for _, candidate := range seen {
		items = append(items, candidate)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].priority == items[j].priority {
			return items[i].relativePath < items[j].relativePath
		}
		return items[i].priority < items[j].priority
	})
	return items
}

func expandPattern(ctx context.Context, root string, pattern string, matcher *workspace.IgnoreMatcher, visit func(string)) error {
	trimmed := filepath.ToSlash(strings.TrimSpace(pattern))
	if trimmed == "" {
		return nil
	}
	if strings.HasSuffix(trimmed, "/") {
		dir := ResolveDocumentPath(root, strings.TrimSuffix(trimmed, "/"))
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() {
					if matcher != nil && matcher.ShouldIgnore(root, path) {
						return filepath.SkipDir
					}
					return nil
				}
				if matcher != nil && matcher.ShouldIgnore(root, path) {
					return nil
				}
				if strings.EqualFold(filepath.Ext(path), ".md") {
					visit(path)
				}
				return nil
			})
		}
		return nil
	}
	if strings.ContainsAny(trimmed, "*?[") {
		matches, err := filepath.Glob(ResolveDocumentPath(root, trimmed))
		if err != nil {
			return err
		}
		for _, match := range matches {
			if err := ctx.Err(); err != nil {
				return err
			}
			if matcher != nil && matcher.ShouldIgnore(root, match) {
				continue
			}
			visit(match)
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	absPath := ResolveDocumentPath(root, trimmed)
	if matcher != nil && matcher.ShouldIgnore(root, absPath) {
		return nil
	}
	visit(absPath)
	return nil
}

// maskWikiContractFrontMatter preserves byte offsets while hiding the
// canonical links envelope from legacy Markdown/reference extraction.
func maskWikiContractFrontMatter(content string) string {
	leading := len(content) - len(strings.TrimLeft(content, " \t\r\n"+string([]byte{0xEF, 0xBB, 0xBF})))
	if leading >= len(content) || !strings.HasPrefix(content[leading:], "---") {
		return content
	}
	endRel := strings.Index(content[leading+3:], "\n---")
	if endRel < 0 {
		return content
	}
	end := leading + 3 + endRel + len("\n---")
	masked := []byte(content)
	for i := leading; i < end; i++ {
		if masked[i] != '\n' && masked[i] != '\r' {
			masked[i] = ' '
		}
	}
	return string(masked)
}

func extractReferences(root string, docPath string, content string) ([]model.DocMention, []model.DocEdge) {
	mentions := make([]model.DocMention, 0)
	edges := make([]model.DocEdge, 0)
	seenMentions := map[string]struct{}{}
	seenEdges := map[string]struct{}{}
	addMention := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := kind + "::" + value
		if _, ok := seenMentions[key]; ok {
			return
		}
		seenMentions[key] = struct{}{}
		mentions = append(mentions, model.DocMention{DocPath: docPath, MentionType: kind, MentionValue: value})
	}
	addEdge := func(edge model.DocEdge) {
		key := edge.FromPath + "::" + edge.Kind + "::" + edge.ToPath + "::" + edge.ToDocID + "::" + edge.Label
		if _, ok := seenEdges[key]; ok {
			return
		}
		seenEdges[key] = struct{}{}
		edges = append(edges, edge)
	}

	masked := maskFencedMarkdown(content)
	for _, match := range docidentity.Extract(masked) {
		addMention(model.DocMentionTypeDocID, match)
		addEdge(model.DocEdge{FromPath: docPath, ToDocID: match, Kind: "doc_id", Label: match})
	}
	for _, indexes := range markdownLinkPattern.FindAllStringSubmatchIndex(masked, -1) {
		if len(indexes) < 4 || indexes[2] < 0 {
			continue
		}
		rawTarget := strings.TrimSpace(content[indexes[2]:indexes[3]])
		target, anchor, ok := parseLocalMarkdownTarget(rawTarget)
		if !ok {
			continue
		}
		if anchor != "" {
			addMention(model.DocMentionTypeAnchor, anchor)
		}
		if target == "" {
			continue
		}
		target = ensureMarkdownDocExtension(target)
		addMention(model.DocMentionTypeDocPath, target)
		addEdge(model.DocEdge{FromPath: docPath, ToPath: target, Kind: "doc_markdown_link", Label: rawTarget})
	}
	for _, indexes := range wikiLinkPattern.FindAllStringSubmatchIndex(masked, -1) {
		if len(indexes) < 4 || indexes[2] < 0 {
			continue
		}
		rawSyntax := content[indexes[0]:indexes[1]]
		inner := strings.TrimSpace(content[indexes[2]:indexes[3]])
		target, anchor, alias, ok := parseLocalWikilink(inner)
		if !ok {
			continue
		}
		if anchor != "" {
			addMention(model.DocMentionTypeAnchor, anchor)
		}
		if alias != "" {
			addMention(model.DocMentionTypeAlias, alias)
		}
		if target == "" {
			continue
		}
		kind := "doc_wikilink"
		if strings.HasPrefix(rawSyntax, "!") {
			kind = "doc_embed"
		}
		addMention(model.DocMentionTypeDocPath, target)
		addEdge(model.DocEdge{FromPath: docPath, ToPath: target, Kind: kind, Label: inner})
	}
	// Semantic relations are intentionally parsed from Markdown list items rather
	// than frontmatter. The link syntax is still parsed by the same local target
	// helpers used by structural document edges.
	frontMatter := false
	firstLine := true
	for _, line := range strings.Split(masked, "\n") {
		trimmed := strings.TrimSpace(line)
		if firstLine {
			if trimmed == "" {
				continue
			}
			firstLine = false
			frontMatter = trimmed == "---"
			continue
		}
		if frontMatter {
			if trimmed == "---" || trimmed == "..." {
				frontMatter = false
			}
			continue
		}
		match := semanticRelationPattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		relation := strings.ToLower(strings.TrimSpace(match[1]))
		link := strings.TrimSpace(match[2])
		var target, label string
		var toDocID string
		if strings.HasPrefix(link, "[[") && strings.HasSuffix(link, "]]") {
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(link, "[["), "]]"))
			parsedTarget, _, _, ok := parseLocalWikilink(inner)
			if !ok || parsedTarget == "" {
				continue
			}
			label = inner
			docIDTarget := strings.TrimSuffix(parsedTarget, ".md")
			matches := docidentity.Extract(docIDTarget)
			if len(matches) == 1 && strings.EqualFold(matches[0], docIDTarget) {
				toDocID = docIDTarget
			} else {
				target = parsedTarget
			}
		} else if strings.HasPrefix(link, "[") {
			open := strings.Index(link, "](")
			if open <= 0 || !strings.HasSuffix(link, ")") {
				continue
			}
			rawTarget := strings.TrimSpace(link[open+2 : len(link)-1])
			parsedTarget, _, ok := parseLocalMarkdownTarget(rawTarget)
			if !ok || parsedTarget == "" {
				continue
			}
			target = ensureMarkdownDocExtension(parsedTarget)
			label = rawTarget
		} else {
			continue
		}
		if target != "" {
			if normalizeDocCandidate(target) == normalizeDocCandidate(docPath) {
				continue
			}
		}
		addEdge(model.DocEdge{FromPath: docPath, ToPath: target, ToDocID: toDocID, Kind: "doc_" + relation, Label: label})
	}
	for _, indexes := range inlineCodePattern.FindAllStringSubmatchIndex(masked, -1) {
		if len(indexes) < 4 || indexes[2] < 0 {
			continue
		}
		value := strings.TrimSpace(content[indexes[2]:indexes[3]])
		switch {
		case strings.HasPrefix(value, "mi-lsp "):
			addMention("command", value)
		case strings.Contains(value, "/") || strings.Contains(value, "\\"):
			normalized := filepath.ToSlash(value)
			if likelyCodePath(normalized) {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(normalized))); err == nil {
					addMention("file_path", normalized)
				}
			}
		default:
			if pascalSymbolPattern.MatchString(value) {
				for _, symbol := range pascalSymbolPattern.FindAllString(value, -1) {
					addMention("symbol", symbol)
				}
			}
		}
	}
	return mentions, edges
}

func parseLocalMarkdownTarget(raw string) (target, anchor string, ok bool) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
		raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "<"), ">"))
	}
	if raw == "" || strings.HasPrefix(raw, "//") {
		return "", "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "", "", false
	}
	anchor, err = url.PathUnescape(parsed.Fragment)
	if err != nil {
		anchor = parsed.Fragment
	}
	target, err = url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return "", "", false
	}
	target = strings.ReplaceAll(strings.TrimSpace(target), "\\", "/")
	return target, anchor, true
}

func parseLocalWikilink(inner string) (target, anchor, alias string, ok bool) {
	targetPart := strings.TrimSpace(inner)
	if pipe := strings.Index(targetPart, "|"); pipe >= 0 {
		alias = strings.TrimSpace(targetPart[pipe+1:])
		targetPart = strings.TrimSpace(targetPart[:pipe])
	}
	if hash := strings.Index(targetPart, "#"); hash >= 0 {
		anchor = strings.TrimSpace(targetPart[hash+1:])
		targetPart = strings.TrimSpace(targetPart[:hash])
	}
	if targetPart == "" {
		return "", anchor, alias, anchor != ""
	}
	if strings.HasPrefix(targetPart, "//") {
		return "", "", "", false
	}
	parsed, err := url.Parse(targetPart)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", "", false
	}
	targetPart, err = url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return "", "", "", false
	}
	targetPart = ensureMarkdownDocExtension(strings.ReplaceAll(strings.TrimSpace(targetPart), "\\", "/"))
	return targetPart, anchor, alias, targetPart != ""
}

func ensureMarkdownDocExtension(target string) string {
	if target == "" || strings.HasSuffix(target, "/") || pathpkg.Ext(target) != "" {
		return target
	}
	return target + ".md"
}

func maskFencedMarkdown(content string) string {
	masked := []byte(content)
	inFence := false
	var fence byte
	minimum := 0
	for start := 0; start < len(masked); {
		end := start
		for end < len(masked) && masked[end] != '\n' {
			end++
		}
		lineEnd := end
		if lineEnd > start && masked[lineEnd-1] == '\r' {
			lineEnd--
		}
		marker, count, rest, markerOK := markdownFence(string(masked[start:lineEnd]))
		maskLine := inFence
		if !inFence && markerOK {
			inFence, fence, minimum, maskLine = true, marker, count, true
		} else if inFence && markerOK && marker == fence && count >= minimum && strings.TrimSpace(rest) == "" {
			inFence, maskLine = false, true
		}
		if maskLine {
			for i := start; i < end; i++ {
				if masked[i] != '\r' {
					masked[i] = ' '
				}
			}
		}
		if end < len(masked) {
			end++
		}
		start = end
	}
	return string(masked)
}

func markdownFence(line string) (marker byte, count int, rest string, ok bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, "", false
	}
	marker = trimmed[0]
	for count < len(trimmed) && trimmed[count] == marker {
		count++
	}
	if count < 3 {
		return 0, 0, "", false
	}
	return marker, count, trimmed[count:], true
}

type docResolutionIndex struct {
	exactPath  map[string]string
	foldedPath map[string][]string
	exactBase  map[string][]string
	foldedBase map[string][]string
	docIDs     map[string][]string
}

func resolveDocEdges(docs []model.DocRecord, edges []model.DocEdge, roots []string) []model.DocEdge {
	index := newDocResolutionIndex(docs)
	resolved := make([]model.DocEdge, 0, len(edges))
	for _, edge := range edges {
		edge.Candidates = nil
		edge.UnresolvedReason = ""
		if edge.Kind == "code" {
			resolved = append(resolved, edge)
			continue
		}
		var candidates []string
		if edge.ToDocID != "" {
			candidates = uniqueSortedDocPaths(index.docIDs[edge.ToDocID])
		} else if edge.ToPath != "" {
			candidates = index.resolvePath(edge.FromPath, edge.ToPath, roots)
			if len(candidates) == 0 && semanticDocumentRelation(edge.Kind) {
				id := strings.TrimSuffix(pathpkg.Base(normalizeDocCandidate(edge.ToPath)), ".md")
				if id != "" {
					candidates = uniqueSortedDocPaths(index.docIDs[id])
				}
			}
		}
		switch len(candidates) {
		case 1:
			if candidates[0] == edge.FromPath {
				continue
			}
			edge.ToPath = candidates[0]
		case 0:
			edge.UnresolvedReason = "missing_doc_target"
		default:
			edge.Candidates = candidates
			edge.UnresolvedReason = "ambiguous_doc_target"
		}
		resolved = append(resolved, edge)
	}
	return resolved
}

func newDocResolutionIndex(docs []model.DocRecord) docResolutionIndex {
	index := docResolutionIndex{
		exactPath:  make(map[string]string, len(docs)),
		foldedPath: make(map[string][]string, len(docs)),
		exactBase:  make(map[string][]string),
		foldedBase: make(map[string][]string),
		docIDs:     make(map[string][]string),
	}
	for _, doc := range docs {
		p := normalizeDocCandidate(doc.Path)
		if p == "" {
			continue
		}
		index.exactPath[p] = p
		index.foldedPath[strings.ToLower(p)] = append(index.foldedPath[strings.ToLower(p)], p)
		base := pathpkg.Base(p)
		index.exactBase[base] = append(index.exactBase[base], p)
		index.foldedBase[strings.ToLower(base)] = append(index.foldedBase[strings.ToLower(base)], p)
		if doc.DocID != "" {
			index.docIDs[doc.DocID] = append(index.docIDs[doc.DocID], p)
		}
	}
	return index
}

func (index docResolutionIndex) resolvePath(fromPath, rawTarget string, roots []string) []string {
	rawTarget = ensureMarkdownDocExtension(strings.ReplaceAll(strings.TrimSpace(rawTarget), "\\", "/"))
	stages := []string{rawTarget}
	if dir := pathpkg.Dir(filepath.ToSlash(fromPath)); dir != "." && dir != "" {
		stages = append(stages, pathpkg.Join(dir, rawTarget))
	}
	for _, root := range roots {
		stages = append(stages, pathpkg.Join(strings.TrimSuffix(filepath.ToSlash(root), "/"), rawTarget))
	}
	seen := map[string]struct{}{}
	for _, candidate := range stages {
		candidate = normalizeDocCandidate(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if exact, ok := index.exactPath[candidate]; ok {
			return []string{exact}
		}
		if folded := uniqueSortedDocPaths(index.foldedPath[strings.ToLower(candidate)]); len(folded) > 0 {
			return folded
		}
	}
	base := pathpkg.Base(normalizeDocCandidate(rawTarget))
	if base == "." || base == "" {
		return nil
	}
	if exact := uniqueSortedDocPaths(index.exactBase[base]); len(exact) > 0 {
		return exact
	}
	return uniqueSortedDocPaths(index.foldedBase[strings.ToLower(base)])
}

func semanticDocumentRelation(kind string) bool {
	switch kind {
	case "fulfills", "implements", "verifies", "affects", "supports", "materializes", "supersedes", "experiences", "stages", "maps", "grounds_in", "doc_related", "doc_depends", "doc_supports", "doc_contradicts", "doc_supersedes":
		return true
	default:
		return false
	}
}

func normalizeDocCandidate(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "/")
	cleaned := pathpkg.Clean(value)
	if cleaned == "." || cleaned == "" || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}

func uniqueSortedDocPaths(paths []string) []string {
	out := append([]string(nil), paths...)
	sort.Strings(out)
	result := make([]string, 0, len(out))
	for _, p := range out {
		if p != "" && (len(result) == 0 || result[len(result)-1] != p) {
			result = append(result, p)
		}
	}
	return result
}

func appendStructuralDocEdges(docs []model.DocRecord, edges []model.DocEdge) []model.DocEdge {
	paths := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		path := filepath.ToSlash(strings.TrimSpace(doc.Path))
		if path == "" {
			continue
		}
		paths[path] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, edge := range edges {
		key := edge.FromPath + "::" + edge.Kind + "::" + edge.ToPath
		seen[key] = struct{}{}
	}
	add := func(from, to, kind string) {
		if from == "" || to == "" || from == to {
			return
		}
		if _, ok := paths[to]; !ok {
			return
		}
		key := from + "::" + kind + "::" + to
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		edges = append(edges, model.DocEdge{FromPath: from, ToPath: to, Kind: kind, Label: kind})
	}
	gobierno := ""
	for candidate := range paths {
		base := strings.ToLower(filepath.Base(candidate))
		if strings.HasPrefix(candidate, "wiki/") && strings.HasPrefix(base, "00-gobierno") && strings.HasSuffix(base, ".md") {
			gobierno = candidate
			break
		}
	}
	for path := range paths {
		dir := filepath.ToSlash(filepath.Dir(path))
		readme := "README.md"
		if dir != "." && dir != "" {
			readme = dir + "/README.md"
		}
		if path != readme {
			add(path, readme, "doc_hierarchy")
		}
		if gobierno != "" && path != gobierno && strings.HasPrefix(path, "wiki/") && !strings.Contains(strings.TrimPrefix(path, "wiki/"), "/") {
			add(path, gobierno, "doc_hierarchy")
		}
	}
	return edges
}

func likelyCodePath(value string) bool {
	ext := strings.ToLower(filepath.Ext(value))
	switch ext {
	case ".go", ".cs", ".ts", ".tsx", ".js", ".jsx", ".py", ".md", ".toml":
		return true
	default:
		return strings.Contains(value, "/") && !strings.Contains(value, " ")
	}
}

func DetectLayerForPath(profile model.DocsReadProfile, path string) string {
	if layer := governanceLayerForPath(profile, path); layer != "" {
		return layer
	}
	return detectLayer(path)
}

func governanceLayerForPath(profile model.DocsReadProfile, path string) string {
	normalized := filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(path), "./"))
	if normalized == "" {
		return ""
	}
	type candidate struct {
		layer string
		score int
	}
	best := candidate{}
	for _, item := range profile.Governance.Hierarchy {
		layer := strings.TrimSpace(item.Layer)
		if layer == "" {
			continue
		}
		for _, pattern := range item.Paths {
			if score := governancePathMatchScore(normalized, pattern); score > best.score {
				best = candidate{layer: normalizeGovernanceLayer(item), score: score}
			}
		}
	}
	return best.layer
}

func normalizeGovernanceLayer(item model.GovernanceHierarchyItem) string {
	stage := strings.ToLower(strings.TrimSpace(item.PackStage))
	id := strings.ToLower(strings.TrimSpace(item.ID))
	if stage == "outcome" || id == "outcome" || id == "resultados" {
		return "RS"
	}
	return strings.TrimSpace(item.Layer)
}

func governancePathMatchScore(path string, pattern string) int {
	pattern = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(pattern), "./"))
	if pattern == "" {
		return 0
	}
	if strings.EqualFold(path, pattern) {
		return 10000 + len(pattern)
	}
	if strings.HasSuffix(pattern, "/") {
		prefix := strings.TrimSuffix(pattern, "/") + "/"
		if strings.HasPrefix(path, prefix) {
			return 8000 + len(prefix)
		}
		return 0
	}
	if strings.ContainsAny(pattern, "*?[") {
		if matched, err := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(path)); err == nil && matched {
			return 6000 + literalPatternLength(pattern)
		}
	}
	return 0
}

func literalPatternLength(pattern string) int {
	count := 0
	for _, r := range pattern {
		switch r {
		case '*', '?', '[', ']':
			continue
		default:
			count++
		}
	}
	return count
}

func detectLayer(path string) string {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)
	lower := strings.ToLower(path)
	lowerBase := strings.ToLower(base)
	if strings.HasPrefix(strings.ToUpper(base), "RS-") ||
		lowerBase == "02_resultados_soluciones_usuario.md" ||
		strings.Contains(lower, "/02_resultados/") ||
		strings.Contains(lower, "/02_outcomes/") {
		return "RS"
	}
	if len(base) >= 2 && base[0] >= '0' && base[0] <= '9' && base[1] >= '0' && base[1] <= '9' {
		return base[:2]
	}
	switch {
	case strings.Contains(path, "/04_flows/") || strings.Contains(path, "/03_FL/"):
		return "04"
	case strings.Contains(path, "/05_requirements/") || strings.Contains(path, "/04_RF/"):
		return "05"
	case strings.Contains(path, "/06_tests/") || strings.Contains(path, "/06_pruebas/"):
		return "06"
	case strings.Contains(path, "/07_technical/") || strings.Contains(path, "/07_tech/"):
		return "07"
	case strings.Contains(path, "/07_technical/db/") || strings.Contains(path, "/08_db/"):
		return "07"
	case strings.Contains(path, "/07_technical/contracts/") || strings.Contains(path, "/09_contratos/"):
		return "07"
	case strings.Contains(path, "/ae/"):
		return "AE"
	default:
		return "generic"
	}
}

func extractTitle(content []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}

func extractSnippet(content []byte) string {
	text := strings.ReplaceAll(string(content), "\r", "")
	lines := strings.Split(text, "\n")
	parts := make([]string, 0, 4)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "```") {
			continue
		}
		parts = append(parts, line)
		if len(strings.Join(parts, " ")) >= 200 {
			break
		}
	}
	snippet := strings.Join(parts, " ")
	if len(snippet) > 220 {
		return strings.TrimSpace(snippet[:220]) + "..."
	}
	return snippet
}

func normalizeSearchText(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	return strings.Join(strings.Fields(value), " ")
}

func firstDocID(value string) string {
	matches := docidentity.Extract(value)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// legacyDocumentID preserves the narrow pre-envelope convention for older
// documents: a known ID may be the complete H1 (or H1 prefix separated by a
// title delimiter), or the complete filename stem. Arbitrary body mentions
// and prose such as "See RF-X" are never ownership evidence.
func legacyDocumentID(relPath, title string) string {
	trimmedTitle := strings.TrimSpace(title)
	matches := docidentity.Extract(trimmedTitle)
	if len(matches) > 0 && strings.HasPrefix(strings.ToLower(trimmedTitle), strings.ToLower(matches[0])) {
		match := matches[0]
		candidate := strings.TrimSpace(trimmedTitle[len(match):])
		if candidate == "" || strings.HasPrefix(candidate, "-") || strings.HasPrefix(candidate, ":") || strings.HasPrefix(candidate, "|") || strings.HasPrefix(candidate, "—") || strings.HasPrefix(candidate, "–") {
			return match
		}
	}
	base := strings.TrimSuffix(pathpkg.Base(filepath.ToSlash(strings.TrimSpace(relPath))), filepath.Ext(filepath.ToSlash(strings.TrimSpace(relPath))))
	if matches := docidentity.Extract(base); len(matches) == 1 && strings.EqualFold(matches[0], base) {
		return base
	}
	return ""
}

// authoritativeDocumentID extracts only an owner identity. Body-wide ID
// scanning is deliberately excluded: references, examples, and links belong
// to extractReferences, not to document identity.
func authoritativeDocumentID(relPath string, content []byte) string {
	if declared := wikisource.DeclaredDocID(string(content)); declared != "" {
		return declared
	}
	if title := extractLeadingTitle(content); title != "" {
		if exact := canonicalOwnerDocID(title); exact != "" {
			return exact
		}
		if prefix := canonicalOwnerDocIDTitlePrefix(title); prefix != "" {
			return prefix
		}
	}
	base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(relPath)), filepath.Ext(filepath.ToSlash(relPath)))
	return canonicalOwnerDocID(base)
}

func extractLeadingTitle(content []byte) string {
	text := strings.TrimPrefix(strings.ReplaceAll(string(content), "\r", ""), "\ufeff")
	lines := strings.Split(text, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start < len(lines) && strings.TrimSpace(lines[start]) == "---" {
		start++
		for start < len(lines) && strings.TrimSpace(lines[start]) != "---" {
			start++
		}
		if start >= len(lines) {
			return ""
		}
		start++
		for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
			start++
		}
	}
	if start >= len(lines) || strings.HasPrefix(strings.TrimSpace(lines[start]), "```") {
		return ""
	}
	trimmed := strings.TrimSpace(lines[start])
	if strings.HasPrefix(trimmed, "#") {
		return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
	}
	return ""
}

func canonicalOwnerDocID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if match := docIDPattern.FindString(value); match == value {
		return value
	}
	return ""
}

func canonicalOwnerDocIDTitlePrefix(value string) string {
	value = strings.TrimSpace(value)
	match := docIDPattern.FindString(value)
	if match == "" || !strings.HasPrefix(value, match) {
		return ""
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(value, match))
	if remainder == "" {
		return match
	}
	for _, separator := range []string{"-", "—", ":"} {
		if strings.HasPrefix(remainder, separator) {
			return match
		}
	}
	return ""
}

func MatchFamily(question string, profile model.DocsReadProfile) string {
	normalized := normalizeSearchText(question)
	bestFamily := "technical"
	bestScore := -1
	for _, family := range profile.Families {
		score := 0
		for _, keyword := range family.IntentKeywords {
			keyword = normalizeSearchText(keyword)
			if keyword != "" && strings.Contains(normalized, keyword) {
				score += 3
			}
		}
		if score > bestScore {
			bestScore = score
			bestFamily = family.Name
		}
	}
	return bestFamily
}

func QuestionTokens(question string) []string {
	normalized := normalizeSearchText(question)
	parts := strings.Fields(normalized)
	stopwords := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "this": {}, "that": {},
		"como": {}, "para": {}, "donde": {}, "porque": {}, "sobre": {}, "desde": {},
		"una": {}, "uno": {}, "que": {}, "del": {}, "las": {}, "los": {}, "con": {},
	}
	shortCanonicalTokens := map[string]struct{}{
		"rf":   {},
		"fl":   {},
		"tp":   {},
		"ct":   {},
		"db":   {},
		"api":  {},
		"sdk":  {},
		"ux":   {},
		"ui":   {},
		"oidc": {},
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, ".,;:!?()[]{}\"'")
		if part == "" {
			continue
		}
		if _, ok := stopwords[part]; ok {
			continue
		}
		if len(part) < 3 {
			if _, ok := shortCanonicalTokens[part]; !ok {
				continue
			}
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func extractFrontMatter(content []byte) *RFFrontMatter {
	text := string(content)
	if !strings.HasPrefix(strings.TrimSpace(text), "---") {
		return nil
	}
	text = strings.TrimSpace(text)
	rest := text[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return nil
	}
	yamlContent := rest[:endIdx]

	var fm RFFrontMatter
	if match := regexp.MustCompile(`(?m)^id:\s*(.+)`).FindStringSubmatch(yamlContent); len(match) > 1 {
		fm.ID = strings.TrimSpace(match[1])
	}
	if match := regexp.MustCompile(`(?m)^title:\s*(.+)`).FindStringSubmatch(yamlContent); len(match) > 1 {
		fm.Title = strings.TrimSpace(match[1])
	}
	fm.Implements = parseYAMLArray(yamlContent, "implements")
	fm.Tests = parseYAMLArray(yamlContent, "tests")

	if fm.ID == "" && len(fm.Implements) == 0 && len(fm.Tests) == 0 {
		return nil
	}
	return &fm
}

func parseYAMLArray(yamlContent string, key string) []string {
	lines := strings.Split(yamlContent, "\n")
	var result []string
	inArray := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+":") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, key+":"))
			if value != "" && strings.HasPrefix(value, "[") {
				if strings.HasSuffix(value, "]") {
					for _, item := range strings.Split(strings.TrimSpace(value[1:len(value)-1]), ",") {
						item = strings.Trim(strings.TrimSpace(item), "\"'")
						if item != "" {
							result = append(result, item)
						}
					}
				}
				return result
			}
			if value != "" {
				result = append(result, value)
				return result
			}
			inArray = true
			continue
		}
		if inArray {
			if strings.HasPrefix(trimmed, "- ") {
				item := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), "\"'")
				if item != "" {
					result = append(result, item)
				}
			} else if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				return result
			}
		}
	}
	return result
}

func digest(content []byte) string {
	sum := sha1.Sum(content)
	return hex.EncodeToString(sum[:])
}
