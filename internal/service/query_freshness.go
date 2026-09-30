package service

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const queryFreshnessRefreshTimeout = time.Second
const queryFreshnessMaxFiles = 32
const queryFreshnessMaxFileBytes int64 = 4 * 1024 * 1024
const queryFreshnessMaxBytes int64 = 16 * 1024 * 1024

// RefreshQueryPaths incrementally refreshes queried files whose disk content
// differs from the published catalog. Refresh work runs synchronously under a
// deadline so a caller never observes a partially completed background write.
func RefreshQueryPaths(ctx context.Context, workspaceRoot string, paths []string, timeout time.Duration) (bool, error) {
	if ctx == nil {
		return false, errors.New("query refresh context is required")
	}
	if timeout <= 0 || len(paths) == 0 {
		return false, nil
	}
	refreshCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	root, err := filepath.Abs(strings.TrimSpace(workspaceRoot))
	if err != nil || strings.TrimSpace(workspaceRoot) == "" {
		return false, fmt.Errorf("query refresh workspace root is invalid")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return false, fmt.Errorf("query refresh workspace root is unavailable: %w", err)
	}
	if err := refreshCtx.Err(); err != nil {
		return false, err
	}

	indexPath := store.WorkspaceDBPath(root)
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	dbStarted := time.Now()
	db, err := store.OpenReadOnlyExistingWithWAL(root, indexPath)
	traceServiceTiming("query_freshness_db_open", dbStarted)
	if err != nil {
		return false, err
	}

	scanStarted := time.Now()
	stale, err := staleQueryPaths(refreshCtx, db, root, paths)
	closeErr := db.Close()
	traceServiceTiming("query_freshness_scan", scanStarted)
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	if err := refreshCtx.Err(); err != nil {
		return false, err
	}
	if len(stale) == 0 {
		return false, nil
	}
	indexStarted := time.Now()
	allCodePaths := true
	for _, path := range stale {
		if strings.EqualFold(filepath.Ext(path), ".md") {
			allCodePaths = false
			break
		}
	}
	var result indexer.Result
	if allCodePaths {
		result, err = indexer.RefreshCatalogPaths(refreshCtx, root, stale)
	} else {
		result, err = indexer.IncrementalCatalogWithPaths(refreshCtx, root, stale)
	}
	if err != nil {
		traceServiceTiming("query_catalog_refresh", indexStarted)
		return false, err
	}
	traceServiceTiming("query_catalog_refresh", indexStarted)
	if result.Stats.Files == 0 && result.Docs == 0 {
		return false, errors.New("query paths could not be indexed incrementally")
	}
	return true, nil
}

func staleQueryPaths(ctx context.Context, db *sql.DB, root string, paths []string) ([]string, error) {
	stale := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	var totalBytes int64
	for _, rawPath := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relPath, absPath, ok := queryPathWithinRoot(root, rawPath)
		if !ok {
			continue
		}
		if _, exists := seen[relPath]; exists {
			continue
		}
		seen[relPath] = struct{}{}
		if len(seen) > queryFreshnessMaxFiles {
			return nil, fmt.Errorf("query refresh deferred: more than %d files were requested", queryFreshnessMaxFiles)
		}
		info, statErr := os.Stat(absPath)
		missing := os.IsNotExist(statErr)
		if statErr != nil && !missing {
			return nil, statErr
		}
		var indexedHash string
		var err error
		if strings.EqualFold(filepath.Ext(relPath), ".md") {
			err = db.QueryRowContext(ctx, `SELECT content_hash FROM doc_records WHERE path=? LIMIT 1`, relPath).Scan(&indexedHash)
			if errors.Is(err, sql.ErrNoRows) {
				if !missing {
					stale = append(stale, relPath)
				}
				continue
			}
		} else {
			err = db.QueryRowContext(ctx, `SELECT content_hash FROM files WHERE file_path=? LIMIT 1`, relPath).Scan(&indexedHash)
			if errors.Is(err, sql.ErrNoRows) {
				if !missing {
					stale = append(stale, relPath)
				}
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		if missing {
			stale = append(stale, relPath)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > queryFreshnessMaxFileBytes || totalBytes+info.Size() > queryFreshnessMaxBytes {
			return nil, fmt.Errorf("query refresh deferred: scoped files exceed the automatic refresh size budget")
		}
		totalBytes += info.Size()
		content, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				stale = append(stale, relPath)
				continue
			}
			return nil, err
		}
		matches := false
		switch {
		case strings.EqualFold(filepath.Ext(relPath), ".md"):
			if len(indexedHash) != 64 {
				continue
			}
			matches = indexedHash == fmt.Sprintf("%x", sha256.Sum256(content))
		case len(indexedHash) == 40:
			matches = indexedHash == fmt.Sprintf("%x", sha1.Sum(content))
		case len(indexedHash) == 32:
			matches = indexedHash == fmt.Sprintf("%x", md5.Sum(content))
		default:
			// Unknown legacy hash encodings are not enough evidence to rewrite
			// indexed symbols during a query. Preserve the published snapshot.
			continue
		}
		if !matches {
			stale = append(stale, relPath)
		}
	}
	return stale, nil
}

func queryPathWithinRoot(root, rawPath string) (string, string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || strings.ContainsRune(rawPath, 0) {
		return "", "", false
	}
	path := rawPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", "", false
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return queryResolvedPathWithinRoot(canonicalRoot, resolved)
	}
	if !os.IsNotExist(err) {
		return "", "", false
	}
	// Deleted paths no longer resolve through EvalSymlinks. Resolve their
	// nearest existing parent physically, then append the missing lexical tail.
	parent := filepath.Dir(path)
	for {
		if _, statErr := os.Lstat(parent); statErr == nil {
			resolvedParent, resolveErr := filepath.EvalSymlinks(parent)
			if resolveErr != nil {
				return "", "", false
			}
			parentRel, ok := queryRelativeWithinRoot(canonicalRoot, resolvedParent)
			if !ok {
				return "", "", false
			}
			tail, tailErr := filepath.Rel(parent, path)
			if tailErr != nil || filepath.IsAbs(tail) || tail == ".." || strings.HasPrefix(tail, ".."+string(filepath.Separator)) {
				return "", "", false
			}
			rel := filepath.Clean(filepath.Join(parentRel, tail))
			if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", "", false
			}
			return filepath.ToSlash(rel), filepath.Join(canonicalRoot, rel), true
		} else if !os.IsNotExist(statErr) {
			return "", "", false
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", "", false
		}
		parent = next
	}
}

func queryResolvedPathWithinRoot(root, path string) (string, string, bool) {
	rel, ok := queryRelativeWithinRoot(root, path)
	if !ok || rel == "." {
		return "", "", false
	}
	return filepath.ToSlash(rel), path, true
}

func queryRelativeWithinRoot(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Clean(rel), true
}
