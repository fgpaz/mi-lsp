package indexer

import (
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/language"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

var (
	gitPath     string
	gitResolved bool
	gitOnce     sync.Once
)

// resolveGitBinary resolves the git binary from MI_LSP_GIT env var or PATH.
// Logs a warning once if falling back to PATH resolution (SEC-06).
func resolveGitBinary() string {
	gitOnce.Do(func() {
		if envPath := os.Getenv("MI_LSP_GIT"); envPath != "" {
			if _, err := os.Stat(envPath); err == nil {
				gitPath = envPath
				gitResolved = true
				return
			}
		}
		if path, err := exec.LookPath("git"); err == nil {
			gitPath = path
			gitResolved = true
			// Log warning once if we had to fall back to PATH (SEC-06)
			if os.Getenv("MI_LSP_GIT") != "" {
				log.Printf("warning: MI_LSP_GIT not found; resolved git from PATH: %s", path)
			}
			return
		}
		gitResolved = true
	})
	return gitPath
}

// ExtractFileSymbols reads a file and extracts symbols from it.
// Used by the file watcher for incremental indexing.
func ExtractFileSymbols(workspaceRoot string, filePath string, repoID string, repoName string) ([]model.SymbolRecord, string, error) {
	absPath := filePath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workspaceRoot, filepath.FromSlash(filePath))
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, "", err
	}

	relPath := filePath
	if filepath.IsAbs(relPath) {
		if rel, err := filepath.Rel(workspaceRoot, relPath); err == nil {
			relPath = filepath.ToSlash(rel)
		}
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	language := languageFromExt(ext)
	repo := model.WorkspaceRepo{ID: repoID, Name: repoName}
	symbols, _ := ExtractCatalog(workspaceRoot, repo, absPath, content)
	return symbols, language, nil
}

func languageFromExt(ext string) string {
	lang, ok := language.ForPath(ext)
	if ok {
		return lang
	}
	return ""
}

func ResolveRepoFromProjectFile(workspaceRoot string, projectFile model.ProjectFile, filePath string) (string, string) {
	if repo, ok := workspace.FindRepoByFile(projectFile, workspaceRoot, filePath); ok {
		return repo.ID, repo.Name
	}
	if repo, ok := workspace.FindRepo(projectFile, projectFile.Project.DefaultRepo); ok {
		return repo.ID, repo.Name
	}
	return "", ""
}

func gitChangedFiles(ctx context.Context, workspaceRoot string) (changed []string, deleted []string) {
	gitBin := resolveGitBinary()
	if gitBin == "" {
		return []string{}, []string{}
	}
	cmd := exec.CommandContext(ctx, gitBin, "status", "--porcelain")
	processutil.ConfigureNonInteractiveCommand(cmd)
	cmd.Dir = workspaceRoot
	output, err := cmd.Output()
	if err != nil {
		return []string{}, []string{}
	}

	lines := strings.Split(string(output), "\n")
	changed = make([]string, 0)
	deleted = make([]string, 0)

	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		filePath := strings.TrimSpace(line[2:])
		if filePath == "" {
			continue
		}
		filePath = filepath.ToSlash(filePath)
		if strings.Contains(filePath, " -> ") && (strings.Contains(status, "R") || strings.Contains(status, "C")) {
			parts := strings.SplitN(filePath, " -> ", 2)
			if len(parts) == 2 {
				deleted = append(deleted, filepath.ToSlash(strings.TrimSpace(parts[0])))
				changed = append(changed, filepath.ToSlash(strings.TrimSpace(parts[1])))
				continue
			}
		}

		switch status {
		case " M", "M ", "MM", "A ", " A", "AA", "??", "R ", " R", "RM", "RC", "C ", " C":
			changed = append(changed, filePath)
		case " D", "D ", "DD", "RD", "DR":
			deleted = append(deleted, filePath)
		}
	}

	return changed, deleted
}

func IncrementalIndex(ctx context.Context, workspaceRoot string) (Result, error) {
	return IncrementalIndexWithGraphProgress(ctx, workspaceRoot, "", nil, GraphIndexOptions{})
}

// IncrementalIndexWithPaths applies an explicit watcher change set. Unlike
// IncrementalIndex, it does not consult Git status: fsnotify events are the
// authoritative input for this background pass, while query-time overlay and
// reconciliation remain responsible for events that were lost before delivery.
func IncrementalIndexWithPaths(ctx context.Context, workspaceRoot string, changedFiles, deletedFiles []string) (Result, error) {
	return incrementalIndexWithGraphProgressAndChanges(ctx, workspaceRoot, "", nil, GraphIndexOptions{}, nil, &incrementalChangeSet{
		changed: normalizeIncrementalPaths(workspaceRoot, changedFiles),
		deleted: normalizeIncrementalPaths(workspaceRoot, deletedFiles),
	})
}

// IncrementalIndexWithGraphProgress updates the catalog and, when needed,
// republishes a complete graph generation before returning success. Observation
// happens before catalog writes so observer failures preserve the prior graph;
// graph staging and activation retain their own transactional CAS guarantees.
func IncrementalIndexWithGraphProgress(ctx context.Context, workspaceRoot, generationID string, progress ProgressFunc, graphOptions GraphIndexOptions) (Result, error) {
	return incrementalIndexWithGraphProgress(ctx, workspaceRoot, generationID, progress, graphOptions, nil)
}

// IncrementalIndexWithGraphProgressForJob keeps file-row mutation separate from
// the final owner-bound metadata/graph publication transaction. The final
// transaction validates owner, fence, status, and cancellation again.
func IncrementalIndexWithGraphProgressForJob(ctx context.Context, workspaceRoot, generationID, jobID string, fence store.IndexJobFence, progress ProgressFunc, graphOptions GraphIndexOptions) (Result, error) {
	return incrementalIndexWithGraphProgress(ctx, workspaceRoot, generationID, progress, graphOptions, &IndexJobPublication{JobID: jobID, Fence: fence})
}

func incrementalIndexWithGraphProgress(ctx context.Context, workspaceRoot, generationID string, progress ProgressFunc, graphOptions GraphIndexOptions, publication *IndexJobPublication) (Result, error) {
	return incrementalIndexWithGraphProgressAndChanges(ctx, workspaceRoot, generationID, progress, graphOptions, publication, nil)
}

// incrementalChangeSet is intentionally private: the watcher only supplies
// normalized paths and the publication/ownership contract remains in T4.
type incrementalChangeSet struct {
	changed []string
	deleted []string
}

func incrementalIndexWithGraphProgressAndChanges(ctx context.Context, workspaceRoot, generationID string, progress ProgressFunc, graphOptions GraphIndexOptions, publication *IndexJobPublication, changes *incrementalChangeSet) (Result, error) {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	indexPath := filepath.Join(workspaceRoot, ".mi-lsp", "index.db")
	if _, err := os.Stat(indexPath); err != nil {
		return Result{}, fmt.Errorf("index.db not found; fallback to full index")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	needsRecovery, err := docIndexNeedsRecovery(ctx, workspaceRoot)
	if err != nil {
		return Result{}, err
	}
	if needsRecovery {
		return Result{}, fmt.Errorf("canonical docs missing from index; fallback to full index")
	}

	var changedFiles, deletedFiles []string
	if changes != nil {
		changedFiles = append([]string(nil), changes.changed...)
		deletedFiles = append([]string(nil), changes.deleted...)
	} else {
		changedFiles, deletedFiles = gitChangedFiles(ctx, workspaceRoot)
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}
	hasChanges := len(changedFiles) != 0 || len(deletedFiles) != 0
	if requiresFullReindex(changedFiles) || requiresFullReindex(deletedFiles) {
		return Result{}, fmt.Errorf("governance/read-model/config changed; fallback to full index")
	}

	var docChangedPaths, docDeletedPaths, codeChangedPaths, codeDeletedPaths []string
	for _, path := range changedFiles {
		if isExcludedIncrementalPath(path) {
			continue
		}
		if isDocPath(path) {
			docChangedPaths = append(docChangedPaths, path)
		} else {
			// Preserve the existing graph-observation behavior for project/config
			// files such as go.mod; only registry-backed source paths are later
			// mutated in the catalog loop.
			codeChangedPaths = append(codeChangedPaths, path)
		}
	}
	for _, path := range deletedFiles {
		if isExcludedIncrementalPath(path) {
			continue
		}
		if isDocPath(path) {
			docDeletedPaths = append(docDeletedPaths, path)
		} else {
			codeDeletedPaths = append(codeDeletedPaths, path)
		}
	}
	hasChanges = len(docChangedPaths) != 0 || len(docDeletedPaths) != 0 || len(codeChangedPaths) != 0 || len(codeDeletedPaths) != 0
	hasDocChanges := len(docChangedPaths) > 0 || len(docDeletedPaths) > 0
	hasCodeChanges := len(codeChangedPaths) > 0 || len(codeDeletedPaths) > 0
	if strings.TrimSpace(generationID) == "" && (hasDocChanges || hasCodeChanges) {
		generationID = fmt.Sprintf("idxgen-incremental-%d", time.Now().UnixNano())
	}

	registration, err := workspace.DetectWorkspace(workspaceRoot)
	if err != nil {
		if !hasChanges {
			return Result{GraphNotApplicable: true, Warnings: []string{"incremental: no supported graph workspace detected"}, Stats: model.Stats{Ms: time.Since(started).Milliseconds()}}, nil
		}
		return Result{}, fmt.Errorf("detect workspace: %w", err)
	}
	projectFile, err := workspace.LoadProjectTopology(workspaceRoot, registration)
	if err != nil {
		return Result{}, fmt.Errorf("load project: %w", err)
	}
	matcher, err := workspace.LoadIgnoreMatcher(workspaceRoot, projectFile.Ignore.ExtraPatterns)
	if err != nil {
		return Result{}, fmt.Errorf("load ignore matcher: %w", err)
	}

	var graphBatches []model.GraphObservationBatch
	var graphOmissions []model.GraphObservationOmission
	var graphWarnings []string
	var docs []model.DocRecord
	var docEdges []model.DocEdge
	var docMentions []model.DocMention
	graphRepair, catalogGeneration, err := incrementalGraphRepairState(ctx, workspaceRoot, hasCodeChanges)
	if err != nil {
		return Result{}, err
	}
	// A document mutation invalidates the graph facts used by observation. Do
	// not observe or publish a graph until a later run sees the final docs.
	observeGraph := graphRepair && !hasDocChanges
	if observeGraph {
		db, err := store.Open(workspaceRoot)
		if err != nil {
			return Result{}, fmt.Errorf("open database for graph facts: %w", err)
		}
		docs, docEdges, docMentions, err = loadIncrementalGraphFacts(ctx, db)
		_ = db.Close()
		if err != nil {
			return Result{}, err
		}
		graphBatches, graphOmissions, graphWarnings, err = ObserveGraph(ctx, workspaceRoot, projectFile, graphOptions, progress)
		if err != nil {
			if publication == nil {
				if staleErr := markIncrementalGraphStale(ctx, workspaceRoot); staleErr != nil {
					return Result{}, fmt.Errorf("incremental graph observation failed: %w; mark graph stale: %v", err, staleErr)
				}
			}
			return Result{}, fmt.Errorf("incremental graph observation failed: %w", err)
		}
		if len(graphBatches) == 0 && !explicitlyNonGraphProject(projectFile) {
			graphWarnings = append(graphWarnings, "graph observation produced no stageable complete batch; publishing catalog with graph stale")
		}
	}

	processedFiles := 0
	skippedFiles := 0
	processedDocs := 0
	var allSymbols []model.SymbolRecord
	var fileChanges []store.IncrementalFileChange
	var docChanges []store.IncrementalDocChange
	var graphGeneration model.GraphGeneration
	graphCurrent := !hasDocChanges && !graphRepair
	graphNotApplicable := false
	var jobGraphPublication *store.IndexJobGraphPublication

	if err := store.WithWorkspaceWriteLock(workspaceRoot, func() error {
		db, err := store.Open(workspaceRoot)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		for _, relPath := range codeChangedPaths {
			if err := ctx.Err(); err != nil {
				return err
			}
			absPath := filepath.Join(workspaceRoot, filepath.FromSlash(relPath))
			if matcher.ShouldIgnore(workspaceRoot, absPath) {
				if changes != nil {
					return fmt.Errorf("code path ignored for %s", relPath)
				}
				skippedFiles++
				continue
			}
			if languageFromExt(strings.ToLower(filepath.Ext(relPath))) == "" {
				skippedFiles++
				continue
			}
			content, readErr := os.ReadFile(absPath)
			if readErr != nil {
				// A watcher event that cannot be read is not a current input.
				// Return the error so the queue preserves it for retry/diagnostics
				// instead of publishing a generation from a partial batch.
				return fmt.Errorf("code read failed for %s: %w", relPath, readErr)
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			symbols, language, extractErr := ExtractFileSymbols(workspaceRoot, relPath, "", "")
			if extractErr != nil {
				return fmt.Errorf("extract symbols for %s: %w", relPath, extractErr)
			}
			repoID, repoName := ResolveRepoFromProjectFile(workspaceRoot, projectFile, relPath)
			fileChanges = append(fileChanges, store.IncrementalFileChange{
				FilePath: relPath, RepoID: repoID, RepoName: repoName, Language: language,
				ContentHash: fmt.Sprintf("%x", md5.Sum(content)), Symbols: symbols,
			})
			allSymbols = append(allSymbols, symbols...)
			processedFiles++
		}
		for _, relPath := range codeDeletedPaths {
			if err := ctx.Err(); err != nil {
				return err
			}
			if languageFromExt(strings.ToLower(filepath.Ext(relPath))) == "" {
				continue
			}
			fileChanges = append(fileChanges, store.IncrementalFileChange{FilePath: relPath, Deleted: true})
			processedFiles++
		}

		if hasDocChanges {
			docsResult, reconcileErr := ReconcileDocs(ctx, workspaceRoot)
			if reconcileErr != nil {
				return fmt.Errorf("reconcile docs: %w", reconcileErr)
			}
			graphWarnings = append(graphWarnings, docsResult.Warnings...)
			for _, path := range docsResult.ExcludedAlive {
				graphWarnings = append(graphWarnings, fmt.Sprintf("doc excluded: %s", path))
			}
			for _, path := range docsResult.Concurrent {
				graphWarnings = append(graphWarnings, fmt.Sprintf("doc concurrent change: %s", path))
			}
			profile, _, profileWarnings := docgraph.LoadProfile(workspaceRoot)
			graphWarnings = append(graphWarnings, profileWarnings...)
			replacePaths := append([]string(nil), docsResult.New...)
			replacePaths = append(replacePaths, docsResult.Changed...)
			pending, buildWarnings := buildDocChangesContext(ctx, workspaceRoot, replacePaths, model.ParserVersion, docsResult.AuthorityConfigHash, docsResult.StoredStates)
			graphWarnings = append(graphWarnings, buildWarnings...)
			parsedReplacements := make([]store.IncrementalDocChange, 0, len(pending))
			for _, change := range pending {
				if err := ctx.Err(); err != nil {
					return err
				}
				parsedDoc, parsedEdges, parsedMentions, parsedBlocks, parsedRecords, parsedBindings, warning, parseErr := docgraph.ParseSingleDoc(workspaceRoot, change.Path, change.Content, profile)
				if warning != "" {
					graphWarnings = append(graphWarnings, fmt.Sprintf("doc parse warning for %s: %s", change.Path, warning))
				}
				if parseErr != nil {
					graphWarnings = append(graphWarnings, fmt.Sprintf("doc parse failed for %s: %v", change.Path, parseErr))
					continue
				}
				if change.State == nil || change.State.ContentSHA256 == "" || change.State.ContentSHA256 != stableContentSHA256(change.Content) {
					graphWarnings = append(graphWarnings, fmt.Sprintf("doc parse failed for %s: stable content hash mismatch", change.Path))
					continue
				}
				change.Doc = &parsedDoc
				change.Edges = parsedEdges
				change.Mentions = parsedMentions
				change.Blocks = parsedBlocks
				change.Records = parsedRecords
				change.Bindings = parsedBindings
				change.State.Lifecycle = lifecycleFromBindings(parsedBindings, change.State.Lifecycle)
				parsedReplacements = append(parsedReplacements, change)
			}

			deletePaths := make(map[string]struct{}, len(docsResult.Deleted))
			for _, path := range docsResult.Deleted {
				if err := ctx.Err(); err != nil {
					return err
				}
				absPath := filepath.Join(workspaceRoot, filepath.FromSlash(path))
				info, statErr := os.Lstat(absPath)
				if statErr == nil && info != nil {
					graphWarnings = append(graphWarnings, fmt.Sprintf("doc deletion skipped for %s: disk still present (no positive proof)", path))
					continue
				}
				if !os.IsNotExist(statErr) {
					graphWarnings = append(graphWarnings, fmt.Sprintf("doc deletion skipped for %s: cannot prove disk absence: %v", path, statErr))
					continue
				}
				deletePaths[path] = struct{}{}
			}

			replacementPaths := make(map[string]struct{}, len(parsedReplacements))
			for _, change := range parsedReplacements {
				replacementPaths[change.Path] = struct{}{}
			}
			for path := range replacementPaths {
				delete(deletePaths, path)
			}
			if len(parsedReplacements) > 0 || len(deletePaths) > 0 {
				existingDocs, existingEdges, _, factsErr := loadIncrementalGraphFacts(ctx, db)
				if factsErr != nil {
					return factsErr
				}
				effective := make([]model.DocRecord, 0, len(existingDocs)+len(parsedReplacements))
				for _, doc := range existingDocs {
					if doc.IsSnapshot {
						continue
					}
					if state, ok := docsResult.StoredStates[doc.Path]; ok && state.Lifecycle == model.DocLifecycleRetired {
						continue
					}
					if _, deleted := deletePaths[doc.Path]; deleted {
						continue
					}
					if _, replaced := replacementPaths[doc.Path]; replaced {
						continue
					}
					effective = append(effective, doc)
				}
				for _, change := range parsedReplacements {
					effective = append(effective, *change.Doc)
				}
				sort.Slice(effective, func(i, j int) bool { return effective[i].Path < effective[j].Path })
				rawEdges := make([]model.DocEdge, 0)
				for _, change := range parsedReplacements {
					rawEdges = append(rawEdges, change.Edges...)
				}
				// Existing edges are intentionally not re-published here: only
				// outgoing edges owned by changed documents may be replaced.
				_ = existingEdges
				resolved := docgraph.ResolveDocEdges(effective, rawEdges, profile)
				byOwner := make(map[string][]model.DocEdge, len(replacementPaths))
				for _, edge := range resolved {
					if _, changed := replacementPaths[edge.FromPath]; changed {
						byOwner[edge.FromPath] = append(byOwner[edge.FromPath], edge)
					}
				}
				for i := range parsedReplacements {
					parsedReplacements[i].Edges = byOwner[parsedReplacements[i].Path]
				}
			}
			docChanges = append(docChanges, parsedReplacements...)
			deleteKeys := make([]string, 0, len(deletePaths))
			for path := range deletePaths {
				deleteKeys = append(deleteKeys, path)
			}
			sort.Strings(deleteKeys)
			for _, path := range deleteKeys {
				docChanges = append(docChanges, store.IncrementalDocChange{Action: "delete", Path: path, Proof: "disk-absent"})
			}
			processedDocs = len(parsedReplacements)
		}

		if observeGraph && len(graphBatches) != 0 {
			prior, ok, priorErr := store.ActiveGraphGeneration(ctx, db)
			if priorErr != nil {
				return priorErr
			}
			var expectedPrior *model.GraphDigest
			if ok {
				expectedPrior = &prior
			}
			if err := reportProgress(ctx, progress, Progress{Stage: "graph.activate", Files: processedFiles, Symbols: len(allSymbols), Docs: len(docs), Force: true}); err != nil {
				return err
			}
			request := GraphAssemblyRequest{Batches: graphBatches, Docs: docs, DocEdges: docEdges, DocMentions: docMentions, CreatedAt: time.Now().UTC()}
			bundle, assembleErr := AssembleGraphObservationBatches(request)
			if assembleErr != nil {
				return fmt.Errorf("incremental graph staging failed: %w", assembleErr)
			}
			graphGeneration = bundle.Generation
			jobGraphPublication = &store.IndexJobGraphPublication{GenerationID: &bundle.Generation.GenerationID, ExpectedPrior: expectedPrior, PublishedAt: request.CreatedAt, GraphCurrent: true, GraphBundle: &bundle, CatalogGeneration: catalogGeneration}
			graphCurrent = true
		} else if observeGraph {
			graphNotApplicable = true
			graphCurrent = false
		}

		hasFileMutation := len(fileChanges) != 0
		hasDocMutation := len(docChanges) != 0
		if publication != nil {
			switch {
			case hasDocChanges && hasFileMutation:
				return store.PublishIncrementalGenerationForJobWithFileAndDocChanges(ctx, db, publication.JobID, generationID, processedFiles, len(allSymbols), processedDocs, publication.Fence, fileChanges, docChanges)
			case hasDocMutation:
				return store.PublishIncrementalDocsGenerationForJob(ctx, db, publication.JobID, generationID, docChanges, publication.Fence)
			case hasFileMutation:
				if graphCurrent || graphNotApplicable {
					var graph *store.IndexJobGraphPublication
					if graphCurrent {
						graph = jobGraphPublication
					}
					return store.PublishIncrementalGenerationForJobWithChanges(ctx, db, publication.JobID, generationID, processedFiles, len(allSymbols), processedDocs, publication.Fence, fileChanges, graph)
				}
				return store.PublishIncrementalGenerationForJobWithFileAndDocChanges(ctx, db, publication.JobID, generationID, processedFiles, len(allSymbols), processedDocs, publication.Fence, fileChanges, nil)
			case observeGraph && graphCurrent:
				return store.PublishIncrementalGenerationForJobWithChanges(ctx, db, publication.JobID, generationID, 0, 0, len(docs), publication.Fence, nil, jobGraphPublication)
			case hasDocChanges:
				return store.CompleteIncrementalDocsJobForJob(ctx, db, publication.JobID, generationID, publication.Fence)
			default:
				return store.PublishIncrementalGenerationForJobWithChanges(ctx, db, publication.JobID, generationID, 0, 0, 0, publication.Fence, nil, &store.IndexJobGraphPublication{GenerationSkippedReason: "no incremental changes"})
			}
		}

		switch {
		case hasDocMutation && hasFileMutation:
			return store.PublishIncrementalGenerationWithFileAndDocChanges(ctx, db, generationID, processedFiles, len(allSymbols), processedDocs, fileChanges, docChanges)
		case hasDocMutation:
			return store.PublishIncrementalDocsGeneration(ctx, db, generationID, docChanges)
		case hasFileMutation:
			if graphCurrent || graphNotApplicable {
				var graph *store.IndexJobGraphPublication
				if graphCurrent {
					graph = jobGraphPublication
				}
				return store.PublishIncrementalGenerationWithChanges(ctx, db, generationID, processedFiles, len(allSymbols), processedDocs, fileChanges, graph)
			}
			return store.PublishIncrementalGenerationWithFileAndDocChanges(ctx, db, generationID, processedFiles, len(allSymbols), processedDocs, fileChanges, nil)
		case observeGraph && graphCurrent:
			return store.PublishIncrementalGenerationWithChanges(ctx, db, generationID, 0, 0, len(docs), nil, jobGraphPublication)
		default:
			return nil
		}
	}); err != nil {
		return Result{}, err
	}

	warnings := append([]string{}, graphWarnings...)
	warnings = append(warnings, fmt.Sprintf("incremental: processed %d files, skipped %d", processedFiles, skippedFiles))
	result := Result{Files: []model.FileRecord{}, Symbols: allSymbols, Docs: processedDocs, Warnings: warnings, GraphOmissions: graphOmissions, GraphNotApplicable: graphNotApplicable, Stats: model.Stats{Files: processedFiles, Symbols: len(allSymbols), TotalDocs: processedDocs, TotalReturned: processedDocs, Ms: time.Since(started).Milliseconds()}}
	if graphGeneration.GenerationID != (model.GraphDigest{}) {
		result.GraphGenerationID = graphGeneration.GenerationID.String()
		result.GraphBackendManifest = graphGeneration.BackendManifestDigest.String()
	}
	return result, nil
}

func publishIncrementalDocsForPublication(ctx context.Context, db *sql.DB, generationID string, changes []store.IncrementalDocChange, publication *IndexJobPublication) error {
	if publication != nil {
		return store.PublishIncrementalDocsGenerationForJob(ctx, db, publication.JobID, generationID, changes, publication.Fence)
	}
	return store.PublishIncrementalDocsGeneration(ctx, db, generationID, changes)
}

// legacyIncrementalIndexWithGraphProgress is retained only as a source-level
// compatibility reference for the pre-T4 implementation. All callers use the
// hardened implementation above.
func legacyIncrementalIndexWithGraphProgress(ctx context.Context, workspaceRoot, generationID string, progress ProgressFunc, graphOptions GraphIndexOptions, publication *IndexJobPublication) (Result, error) {
	started := time.Now()
	indexPath := filepath.Join(workspaceRoot, ".mi-lsp", "index.db")
	if _, err := os.Stat(indexPath); err != nil {
		return Result{}, fmt.Errorf("index.db not found; fallback to full index")
	}
	needsRecovery, err := docIndexNeedsRecovery(ctx, workspaceRoot)
	if err != nil {
		return Result{}, err
	}
	if needsRecovery {
		return Result{}, fmt.Errorf("canonical docs missing from index; fallback to full index")
	}

	changedFiles, deletedFiles := gitChangedFiles(ctx, workspaceRoot)
	hasChanges := len(changedFiles) != 0 || len(deletedFiles) != 0
	if requiresFullReindex(changedFiles) || requiresFullReindex(deletedFiles) {
		return Result{}, fmt.Errorf("governance/read-model/config changed; fallback to full index")
	}

	registration, err := workspace.DetectWorkspace(workspaceRoot)
	if err != nil {
		if !hasChanges {
			return Result{GraphNotApplicable: true, Warnings: []string{"incremental: no supported graph workspace detected"}, Stats: model.Stats{Ms: time.Since(started).Milliseconds()}}, nil
		}
		return Result{}, fmt.Errorf("detect workspace: %w", err)
	}
	projectFile, err := workspace.LoadProjectTopology(workspaceRoot, registration)
	if err != nil {
		return Result{}, fmt.Errorf("load project: %w", err)
	}
	matcher, err := workspace.LoadIgnoreMatcher(workspaceRoot, projectFile.Ignore.ExtraPatterns)
	if err != nil {
		return Result{}, fmt.Errorf("load ignore matcher: %w", err)
	}

	var graphBatches []model.GraphObservationBatch
	var graphOmissions []model.GraphObservationOmission
	var graphWarnings []string
	var docs []model.DocRecord
	var docEdges []model.DocEdge
	var docMentions []model.DocMention
	graphRepair, catalogGeneration, err := incrementalGraphRepairState(ctx, workspaceRoot, hasChanges)
	if err != nil {
		return Result{}, err
	}
	if graphRepair {
		db, err := store.Open(workspaceRoot)
		if err != nil {
			return Result{}, fmt.Errorf("open database for graph facts: %w", err)
		}
		docs, docEdges, docMentions, err = loadIncrementalGraphFacts(ctx, db)
		_ = db.Close()
		if err != nil {
			return Result{}, err
		}
		graphBatches, graphOmissions, graphWarnings, err = ObserveGraph(ctx, workspaceRoot, projectFile, graphOptions, progress)
		if err != nil {
			if publication == nil {
				if staleErr := markIncrementalGraphStale(ctx, workspaceRoot); staleErr != nil {
					return Result{}, fmt.Errorf("incremental graph observation failed: %w; mark graph stale: %v", err, staleErr)
				}
			}
			return Result{}, fmt.Errorf("incremental graph observation failed: %w", err)
		}
		// All-partial / gated observations yield zero stageable batches; catalog incremental
		// must still proceed with graph stale (see full index path).
		if len(graphBatches) == 0 && !explicitlyNonGraphProject(projectFile) {
			graphWarnings = append(graphWarnings, "graph observation produced no stageable complete batch; publishing catalog with graph stale")
		}
	}

	processedFiles := 0
	skippedFiles := 0
	var allSymbols []model.SymbolRecord
	var fileChanges []store.IncrementalFileChange
	var graphGeneration model.GraphGeneration
	graphCurrent := !graphRepair
	graphNotApplicable := graphRepair && len(graphBatches) == 0
	var jobGraphPublication *store.IndexJobGraphPublication

	// Classify changed and deleted files into doc paths and code paths
	// BEFORE the existing symbol loop so docs and code can be handled separately.
	var docChangedPaths []string
	var docDeletedPaths []string
	var codeChangedPaths []string
	var codeDeletedPaths []string

	for _, p := range changedFiles {
		if isExcludedIncrementalPath(p) {
			continue
		}
		if isDocPath(p) {
			docChangedPaths = append(docChangedPaths, p)
		} else {
			codeChangedPaths = append(codeChangedPaths, p)
		}
	}
	for _, p := range deletedFiles {
		if isExcludedIncrementalPath(p) {
			continue
		}
		if isDocPath(p) {
			docDeletedPaths = append(docDeletedPaths, p)
		} else {
			codeDeletedPaths = append(codeDeletedPaths, p)
		}
	}

	hasDocChanges := len(docChangedPaths) > 0 || len(docDeletedPaths) > 0

	if err := store.WithWorkspaceWriteLock(workspaceRoot, func() error {
		db, err := store.Open(workspaceRoot)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		// === CODE PATH: process code changes (symbols, files) ===
		for _, relPath := range codeChangedPaths {
			absPath := filepath.Join(workspaceRoot, filepath.FromSlash(relPath))
			if matcher.ShouldIgnore(workspaceRoot, absPath) {
				skippedFiles++
				continue
			}
			if languageFromExt(strings.ToLower(filepath.Ext(relPath))) == "" {
				skippedFiles++
				continue
			}
			content, err := os.ReadFile(absPath)
			if err != nil {
				fileChanges = append(fileChanges, store.IncrementalFileChange{FilePath: relPath, Deleted: true})
				processedFiles++
				continue
			}
			symbols, language, err := ExtractFileSymbols(workspaceRoot, relPath, "", "")
			if err != nil {
				return fmt.Errorf("extract symbols for %s: %w", relPath, err)
			}
			repoID, repoName := ResolveRepoFromProjectFile(workspaceRoot, projectFile, relPath)
			contentHash := fmt.Sprintf("%x", md5.Sum(content))
			fileChanges = append(fileChanges, store.IncrementalFileChange{
				FilePath: relPath, RepoID: repoID, RepoName: repoName, Language: language,
				ContentHash: contentHash, Symbols: symbols,
			})
			allSymbols = append(allSymbols, symbols...)
			processedFiles++
		}

		for _, relPath := range codeDeletedPaths {
			if languageFromExt(strings.ToLower(filepath.Ext(relPath))) == "" {
				continue
			}
			fileChanges = append(fileChanges, store.IncrementalFileChange{FilePath: relPath, Deleted: true})
			processedFiles++
		}

		// === DOCS PATH: reconcile and publish doc changes ===
		if hasDocChanges {
			docsResult, err := ReconcileDocs(ctx, workspaceRoot)
			if err != nil {
				return fmt.Errorf("reconcile docs: %w", err)
			}

			// Parse new and changed documents.
			if len(docsResult.New) > 0 || len(docsResult.Changed) > 0 {
				profile, _, _ := docgraph.LoadProfile(workspaceRoot)
				newOrChanged := append([]string(nil), docsResult.New...)
				newOrChanged = append(newOrChanged, docsResult.Changed...)
				docChanges := buildDocChanges(workspaceRoot, newOrChanged, model.ParserVersion, docsResult.AuthorityConfigHash)

				// Parse each change using the bounded single-doc parser.
				for i := range docChanges {
					c := &docChanges[i]
					if c.Action != "replace" {
						continue
					}
					absPath := filepath.Join(workspaceRoot, filepath.FromSlash(c.Path))
					content, err := os.ReadFile(absPath)
					if err != nil {
						graphWarnings = append(graphWarnings, fmt.Sprintf("doc parse failed for %s: %v", c.Path, err))
						c.Doc = nil
						continue
					}
					parsedDoc, parsedEdges, parsedMentions, parsedBlocks, parsedRecords, parsedBindings, warn, parseErr := docgraph.ParseSingleDoc(workspaceRoot, c.Path, content, profile)
					if warn != "" {
						graphWarnings = append(graphWarnings, "doc_parse: "+warn)
					}
					if parseErr != nil {
						graphWarnings = append(graphWarnings, fmt.Sprintf("doc parse error for %s: %v", c.Path, parseErr))
						continue
					}
					c.Doc = &parsedDoc
					c.Edges = parsedEdges
					c.Mentions = parsedMentions
					c.Blocks = parsedBlocks
					c.Records = parsedRecords
					c.Bindings = parsedBindings
				}

				validDocChanges := make([]store.IncrementalDocChange, 0, len(docChanges))
				for _, change := range docChanges {
					if change.Action == "replace" && change.Doc != nil && change.State != nil {
						validDocChanges = append(validDocChanges, change)
					}
				}
				if len(validDocChanges) > 0 {
					if err := publishIncrementalDocsForPublication(ctx, db, generationID, validDocChanges, publication); err != nil {
						return fmt.Errorf("publish incremental docs: %w", err)
					}
				}
			}

			// Handle deletions: positive proof required.
			if len(docsResult.Deleted) > 0 {
				var delChanges []store.IncrementalDocChange
				for _, p := range docsResult.Deleted {
					absPath := filepath.Join(workspaceRoot, filepath.FromSlash(p))
					if store.IsDiskAbsent(absPath) {
						delChanges = append(delChanges, store.IncrementalDocChange{
							Action: "delete",
							Path:   p,
							Proof:  "disk-absent",
						})
					} else {
						graphWarnings = append(graphWarnings, fmt.Sprintf("delete skipped for %s: disk still present (no positive proof)", p))
					}
				}
				if len(delChanges) > 0 {
					if err := publishIncrementalDocsForPublication(ctx, db, generationID, delChanges, publication); err != nil {
						return fmt.Errorf("publish incremental docs delete: %w", err)
					}
				}
			}

			// Retain previous rows for excluded, concurrent, and parse-failed paths.
			for _, p := range docsResult.ExcludedAlive {
				graphWarnings = append(graphWarnings, fmt.Sprintf("doc excluded: %s", p))
			}
			for _, p := range docsResult.Concurrent {
				graphWarnings = append(graphWarnings, fmt.Sprintf("doc concurrent change: %s", p))
			}
		}

		if graphRepair {
			if len(graphBatches) != 0 {
				prior, ok, err := store.ActiveGraphGeneration(ctx, db)
				if err != nil {
					return err
				}
				var expectedPrior *model.GraphDigest
				if ok {
					expectedPrior = &prior
				}
				if err := reportProgress(ctx, progress, Progress{Stage: "graph.activate", Files: processedFiles, Symbols: len(allSymbols), Docs: len(docs), Force: true}); err != nil {
					return err
				}
				request := GraphAssemblyRequest{Batches: graphBatches, Docs: docs, DocEdges: docEdges, DocMentions: docMentions, CreatedAt: time.Now().UTC()}
				bundle, assembleErr := AssembleGraphObservationBatches(request)
				if assembleErr != nil {
					return fmt.Errorf("incremental graph staging failed: %w", assembleErr)
				}
				graphGeneration = bundle.Generation
				jobGraphPublication = &store.IndexJobGraphPublication{GenerationID: &bundle.Generation.GenerationID, ExpectedPrior: expectedPrior, PublishedAt: request.CreatedAt, GraphCurrent: true, GraphBundle: &bundle, CatalogGeneration: catalogGeneration}
				graphCurrent = true
			}
		}

		if publication != nil {
			if graphCurrent {
				if jobGraphPublication == nil {
					jobGraphPublication = &store.IndexJobGraphPublication{GraphCurrent: true}
				}
				if err := store.PublishIncrementalGenerationForJobWithChanges(ctx, db, publication.JobID, generationID, processedFiles, len(allSymbols), len(docs), publication.Fence, fileChanges, jobGraphPublication); err != nil {
					return err
				}
			} else if graphNotApplicable {
				jobGraphPublication = &store.IndexJobGraphPublication{GenerationSkippedReason: "incremental catalog update has no graph-capable backend"}
				if err := store.PublishIncrementalGenerationForJobWithChanges(ctx, db, publication.JobID, generationID, processedFiles, len(allSymbols), len(docs), publication.Fence, fileChanges, jobGraphPublication); err != nil {
					return err
				}
			}
		} else if generationID != "" || graphRepair {
			foregroundGraph := jobGraphPublication
			if graphNotApplicable {
				foregroundGraph = &store.IndexJobGraphPublication{GraphCurrent: false}
			}
			if err := store.PublishIncrementalGenerationWithChanges(ctx, db, generationID, processedFiles, len(allSymbols), len(docs), fileChanges, foregroundGraph); err != nil {
				if graphCurrent {
					_ = store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeStale, "")
				}
				return err
			}
		}
		return nil
	}); err != nil {
		return Result{}, err
	}

	warnings := append([]string{}, graphWarnings...)
	warnings = append(warnings, fmt.Sprintf("incremental: processed %d files, skipped %d", processedFiles, skippedFiles))
	result := Result{Files: []model.FileRecord{}, Symbols: allSymbols, Warnings: warnings, GraphOmissions: graphOmissions, GraphNotApplicable: graphNotApplicable, Stats: model.Stats{Files: processedFiles, Symbols: len(allSymbols), Ms: time.Since(started).Milliseconds()}}
	if graphGeneration.GenerationID != (model.GraphDigest{}) {
		result.GraphGenerationID = graphGeneration.GenerationID.String()
		result.GraphBackendManifest = graphGeneration.BackendManifestDigest.String()
	}
	return result, nil
}

func incrementalGraphRepairState(ctx context.Context, workspaceRoot string, force bool) (bool, string, error) {
	db, err := store.Open(workspaceRoot)
	if err != nil {
		return false, "", fmt.Errorf("open database for graph freshness: %w", err)
	}
	defer db.Close()
	catalogGeneration, _, err := workspaceMetaValue(ctx, db, store.WorkspaceMetaActiveCatalogGeneration)
	if err != nil {
		return false, "", err
	}
	if force {
		return true, catalogGeneration, nil
	}
	freshness, err := store.GraphFreshness(ctx, db, "")
	if err != nil {
		return false, catalogGeneration, err
	}
	if freshness.State != model.GraphFreshnessCurrent {
		return true, catalogGeneration, nil
	}
	if catalogGeneration != "" {
		graphCatalogGeneration, ok, err := workspaceMetaValue(ctx, db, store.GraphCatalogGenerationMeta)
		if err != nil {
			return false, catalogGeneration, err
		}
		if !ok || graphCatalogGeneration != catalogGeneration {
			return true, catalogGeneration, nil
		}
	}
	return false, catalogGeneration, nil
}

func markIncrementalGraphStale(ctx context.Context, workspaceRoot string) error {
	db, err := store.Open(workspaceRoot)
	if err != nil {
		return fmt.Errorf("open database to mark graph stale: %w", err)
	}
	defer db.Close()
	if err := store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeStale, ""); err != nil {
		return fmt.Errorf("mark graph stale: %w", err)
	}
	return nil
}

func workspaceMetaValue(ctx context.Context, db *sql.DB, key string) (string, bool, error) {
	var value string
	err := db.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func loadIncrementalGraphFacts(ctx context.Context, db *sql.DB) ([]model.DocRecord, []model.DocEdge, []model.DocMention, error) {
	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list graph documents: %w", err)
	}
	edges := make([]model.DocEdge, 0)
	rows, err := db.QueryContext(ctx, `SELECT from_path, to_path, to_doc_id, kind, label FROM doc_edges ORDER BY from_path, to_path, to_doc_id, kind, label`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list graph document edges: %w", err)
	}
	for rows.Next() {
		var edge model.DocEdge
		if err := rows.Scan(&edge.FromPath, &edge.ToPath, &edge.ToDocID, &edge.Kind, &edge.Label); err != nil {
			_ = rows.Close()
			return nil, nil, nil, fmt.Errorf("scan graph document edge: %w", err)
		}
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, nil, nil, fmt.Errorf("read graph document edges: %w", err)
	}
	_ = rows.Close()

	mentions, err := store.ListDocMentions(ctx, db)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list graph document mentions: %w", err)
	}
	return docs, edges, mentions, nil
}

func explicitlyNonGraphProject(project model.ProjectFile) bool {
	if len(project.Repos) == 0 {
		return false
	}
	selected := strings.TrimSpace(project.Project.DefaultRepo)
	if selected == "" && len(project.Repos) == 1 {
		selected = project.Repos[0].ID
	}
	for _, repo := range project.Repos {
		if repo.ID != selected {
			continue
		}
		if len(repo.Languages) == 0 {
			return false
		}
		for _, language := range repo.Languages {
			switch strings.ToLower(strings.TrimSpace(language)) {
			case "go", "csharp", "cs", "dotnet":
				return false
			}
		}
		return true
	}
	return false
}

// IsAuthorityConfigPath reports whether a path can change canonical document
// discovery or another high-fanout authority input. The watcher uses the same
// predicate as incremental indexing so a config event cannot be mistaken for
// an ordinary document edit.
func IsAuthorityConfigPath(path string) bool {
	normalized := strings.ToLower(normalizeIncrementalPath(path))
	if normalized == "" || isExcludedIncrementalPath(normalized) {
		return false
	}
	base := filepath.Base(normalized)
	if base == "read-model.toml" && hasPathSegment(normalized, "_mi-lsp") {
		return true
	}
	switch base {
	case "00_gobierno_documental.md", "07_baseline_tecnica.md", ".gitignore", ".gitattributes", ".gitmodules", ".milspignore":
		return true
	case "project.toml":
		return strings.HasPrefix(normalized, ".mi-lsp/")
	}
	// README changes have broad structural fanout in the T4 pipeline.
	return strings.HasPrefix(normalized, "readme") && strings.HasSuffix(normalized, ".md")
}

func requiresFullReindex(paths []string) bool {
	for _, path := range paths {
		if IsAuthorityConfigPath(path) {
			return true
		}
	}
	return false
}

func hasPathSegment(path, want string) bool {
	for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
		if segment == want {
			return true
		}
	}
	return false
}

func isExcludedIncrementalPath(path string) bool {
	parts := strings.Split(strings.ToLower(strings.Trim(normalizeIncrementalPath(path), "/")), "/")
	for index, part := range parts {
		if part == ".mi-lsp" || part == ".git" || part == ".worktrees" {
			return true
		}
		if part == ".docs" && index+1 < len(parts) {
			switch parts[index+1] {
			case "raw", "auditoria", "temp":
				return true
			}
		}
	}
	return false
}

func normalizeIncrementalPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || strings.ContainsRune(path, 0) {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == "" {
		return ""
	}
	return clean
}

func normalizeIncrementalPaths(root string, paths []string) []string {
	absRoot, _ := filepath.Abs(root)
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) {
			rel, err := filepath.Rel(absRoot, filepath.Clean(path))
			if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			path = rel
		}
		path = normalizeIncrementalPath(path)
		if path == "" || path == ".." || strings.HasPrefix(path, "../") || isExcludedIncrementalPath(path) {
			continue
		}
		if !language.IsSupportedCodePath(path) && !isDocPath(path) && !IsAuthorityConfigPath(path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func docIndexNeedsRecovery(ctx context.Context, workspaceRoot string) (bool, error) {
	if !canonicalDocsExistOnDisk(workspaceRoot) {
		return false, nil
	}

	db, err := store.Open(workspaceRoot)
	if err != nil {
		return false, err
	}
	defer db.Close()

	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return false, err
	}
	if len(docs) == 0 {
		return true, nil
	}
	for _, doc := range docs {
		if doc.IsSnapshot {
			continue
		}
		if doc.Family != "" && doc.Family != "generic" {
			return false, nil
		}
	}
	return true, nil
}

func canonicalDocsExistOnDisk(workspaceRoot string) bool {
	for _, relativePath := range []string{
		".docs/wiki/00_gobierno_documental.md",
		".docs/wiki/_mi-lsp/read-model.toml",
		".docs/wiki/03_FL.md",
		".docs/wiki/04_RF.md",
		".docs/wiki/07_baseline_tecnica.md",
		".docs/wiki/09_contratos_tecnicos.md",
	} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, filepath.FromSlash(relativePath))); err == nil {
			return true
		}
	}
	return false
}
