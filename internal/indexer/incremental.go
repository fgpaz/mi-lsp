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
	"strings"
	"sync"
	"time"

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
	switch ext {
	case ".cs":
		return "csharp"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py", ".pyi":
		return "python"
	default:
		return ""
	}
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

		switch status {
		case " M", "M ", "MM", "A ", " A", "AA", "??":
			changed = append(changed, filePath)
		case " D", "D ", "DD":
			deleted = append(deleted, filePath)
		}
	}

	return changed, deleted
}

func IncrementalIndex(ctx context.Context, workspaceRoot string) (Result, error) {
	return IncrementalIndexWithGraphProgress(ctx, workspaceRoot, "", nil, GraphIndexOptions{})
}

// IncrementalIndexWithGraphProgress updates the catalog and, when needed,
// republishes a complete graph generation before returning success. Observation
// happens before catalog writes so observer failures preserve the prior graph;
// graph staging and activation retain their own transactional CAS guarantees.
func IncrementalIndexWithGraphProgress(ctx context.Context, workspaceRoot, generationID string, progress ProgressFunc, graphOptions GraphIndexOptions) (Result, error) {
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
		return Result{}, fmt.Errorf("documentation or read-model changed; fallback to full index")
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
		if err := markIncrementalGraphStale(ctx, workspaceRoot); err != nil {
			return Result{}, err
		}
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
			return Result{}, fmt.Errorf("incremental graph observation failed: %w", err)
		}
		if len(graphBatches) == 0 && !explicitlyNonGraphProject(projectFile) {
			return Result{}, fmt.Errorf("incremental graph observation produced no publishable generation")
		}
	}

	processedFiles := 0
	skippedFiles := 0
	var allSymbols []model.SymbolRecord
	var graphGeneration model.GraphGeneration
	graphCurrent := !graphRepair
	graphNotApplicable := graphRepair && len(graphBatches) == 0
	if err := store.WithWorkspaceWriteLock(workspaceRoot, func() error {
		db, err := store.Open(workspaceRoot)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		for _, relPath := range changedFiles {
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
				if err := store.DeleteFileSymbols(ctx, db, relPath); err != nil {
					return fmt.Errorf("delete symbols for %s: %w", relPath, err)
				}
				processedFiles++
				continue
			}
			symbols, language, err := ExtractFileSymbols(workspaceRoot, relPath, "", "")
			if err != nil {
				return fmt.Errorf("extract symbols for %s: %w", relPath, err)
			}
			repoID, repoName := ResolveRepoFromProjectFile(workspaceRoot, projectFile, relPath)
			contentHash := fmt.Sprintf("%x", md5.Sum(content))
			if err := store.ReplaceFileSymbols(ctx, db, relPath, repoID, repoName, language, contentHash, symbols); err != nil {
				return fmt.Errorf("replace symbols for %s: %w", relPath, err)
			}
			allSymbols = append(allSymbols, symbols...)
			processedFiles++
		}

		for _, relPath := range deletedFiles {
			if languageFromExt(strings.ToLower(filepath.Ext(relPath))) == "" {
				continue
			}
			if err := store.DeleteFileSymbols(ctx, db, relPath); err != nil {
				return fmt.Errorf("delete symbols for %s: %w", relPath, err)
			}
			processedFiles++
		}

		if graphRepair {
			if len(graphBatches) != 0 {
				if err := store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeStale, ""); err != nil {
					return fmt.Errorf("mark graph stale: %w", err)
				}
				prior, ok, err := store.ActiveGraphGeneration(ctx, db)
				if err != nil {
					return err
				}
				if !ok {
					prior, ok, err = restoreDanglingActiveGraphPointer(ctx, db)
					if err != nil {
						return err
					}
				}
				var expectedPrior *model.GraphDigest
				if ok {
					expectedPrior = &prior
				}
				if err := reportProgress(ctx, progress, Progress{Stage: "graph.activate", Files: processedFiles, Symbols: len(allSymbols), Docs: len(docs), Force: true}); err != nil {
					return err
				}
				graphGeneration, err = PublishGraphObservationBatches(ctx, db, GraphAssemblyRequest{Batches: graphBatches, Docs: docs, DocEdges: docEdges, DocMentions: docMentions, CreatedAt: time.Now().UTC()}, expectedPrior)
				if err != nil {
					_ = store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeStale, "")
					return fmt.Errorf("incremental graph publication failed: %w", err)
				}
				graphCurrent = true
			}
		}

		if generationID != "" {
			if err := publishIncrementalGeneration(ctx, db, generationID, processedFiles, len(allSymbols), len(docs), graphCurrent); err != nil {
				if graphCurrent {
					_ = store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeStale, "")
				}
				return err
			}
		} else if graphCurrent && graphRepair {
			if err := store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeFresh, catalogGeneration); err != nil {
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

func restoreDanglingActiveGraphPointer(ctx context.Context, db *sql.DB) (model.GraphDigest, bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT lower(hex(generation_id)) FROM graph_generations WHERE status=? ORDER BY generation_id LIMIT 2", model.GraphGenerationActive)
	if err != nil {
		return model.GraphDigest{}, false, err
	}
	defer rows.Close()
	var ids []model.GraphDigest
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return model.GraphDigest{}, false, err
		}
		id, err := model.ParseGraphDigest(raw)
		if err != nil {
			return model.GraphDigest{}, false, fmt.Errorf("invalid dangling graph generation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return model.GraphDigest{}, false, err
	}
	if len(ids) == 0 {
		return model.GraphDigest{}, false, nil
	}
	if len(ids) != 1 {
		return model.GraphDigest{}, false, model.ErrGraphPointerConflict
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "active_graph_generation_id", ids[0][:]); err != nil {
		return model.GraphDigest{}, false, err
	}
	return ids[0], true, nil
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

	mentions := make([]model.DocMention, 0)
	rows, err = db.QueryContext(ctx, `SELECT doc_path, mention_type, mention_value FROM doc_mentions ORDER BY doc_path, mention_type, mention_value`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list graph document mentions: %w", err)
	}
	for rows.Next() {
		var mention model.DocMention
		if err := rows.Scan(&mention.DocPath, &mention.MentionType, &mention.MentionValue); err != nil {
			_ = rows.Close()
			return nil, nil, nil, fmt.Errorf("scan graph document mention: %w", err)
		}
		mentions = append(mentions, mention)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, nil, nil, fmt.Errorf("read graph document mentions: %w", err)
	}
	_ = rows.Close()
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

func publishIncrementalGeneration(ctx context.Context, db *sql.DB, generationID string, files, symbols, docs int, graphCurrent bool) error {
	if !graphCurrent {
		// Explicitly non-graph work keeps the index generation pending so the
		// caller can mark it skipped without claiming a current graph.
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE index_generations SET status='published', files=?, symbols=?, docs=?, published_at=?, error=NULL WHERE generation_id=?`, files, symbols, docs, now, generationID); err != nil {
		return err
	}
	metadata := map[string]string{
		store.WorkspaceMetaLastIndexGeneration:     generationID,
		store.WorkspaceMetaActiveCatalogGeneration: generationID,
		"active_generation_mode":                   "catalog",
		"active_generation_published_at":           now,
		"active_generation_files":                  fmt.Sprintf("%d", files),
		"active_generation_symbols":                fmt.Sprintf("%d", symbols),
		"active_generation_docs":                   fmt.Sprintf("%d", docs),
	}
	metadata[store.GraphRuntimeStateMeta] = store.GraphRuntimeFresh
	metadata[store.GraphCatalogGenerationMeta] = generationID
	for key, value := range metadata {
		if _, err := tx.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func requiresFullReindex(paths []string) bool {
	for _, path := range paths {
		normalized := filepath.ToSlash(strings.ToLower(path))
		base := filepath.Base(normalized)
		if strings.HasPrefix(normalized, ".docs/") || strings.HasPrefix(normalized, "docs/") {
			return true
		}
		if strings.HasPrefix(normalized, "readme") && strings.HasSuffix(normalized, ".md") {
			return true
		}
		if base == "read-model.toml" && strings.Contains(normalized, ".docs/wiki/_mi-lsp/") {
			return true
		}
	}
	return false
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
