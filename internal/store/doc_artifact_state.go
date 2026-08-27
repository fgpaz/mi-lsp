package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// --- Authority / config digest ---

// authorityConfigDigest computes a deterministic hash from the documents read
// profile configuration. It deliberately excludes absolute workspace paths and
// timestamps so the digest is reproducible across workspace roots.
func authorityConfigDigest(profile model.DocsReadProfile, extraOverlays []string) string {
	payload, err := json.Marshal(profile)
	if err != nil {
		payload = []byte(fmt.Sprintf("%#v", profile))
	}
	h := sha256.New()
	_, _ = h.Write([]byte("mi-lsp-doc-profile-v1\x00"))
	_, _ = h.Write(payload)
	for _, overlay := range extraOverlays {
		_, _ = fmt.Fprintf(h, "\x00%d:", len(overlay))
		_, _ = h.Write([]byte(overlay))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AuthorityConfigDigestForProfile is the profile-aware digest used by the
// incremental document reconciler. A profile change invalidates the document
// parser state even when the file's size and mtime are unchanged.
func AuthorityConfigDigestForProfile(profile model.DocsReadProfile, extraOverlays []string) string {
	return authorityConfigDigest(profile, extraOverlays)
}

// AuthorityConfigDigest retains the project-based compatibility API. Project
// topology and overlays are hashed as a deterministic JSON payload.
func AuthorityConfigDigest(project model.ProjectFile, extraOverlays []string) string {
	payload, err := json.Marshal(project)
	if err != nil {
		payload = []byte(fmt.Sprintf("%#v", project))
	}
	h := sha256.New()
	_, _ = h.Write([]byte("mi-lsp-project-profile-v1\x00"))
	_, _ = h.Write(payload)
	for _, overlay := range extraOverlays {
		_, _ = fmt.Fprintf(h, "\x00%d:", len(overlay))
		_, _ = h.Write([]byte(overlay))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AuthorityConfigHashKey is the workspace_meta key under which the digest is
// stored so downstream code can detect when the authority configuration changes.
const AuthorityConfigHashKey = "docs_authority_config_hash"

// --- Racily-clean helper ---

// racilyCleanStat is the platform's filesystem timestamp granularity in
// nanoseconds. Values are read from runtime.GOOS on first use.
var racilyCleanStat struct {
	sync.Once
	granularity int64
}

// racilyCleanWindow returns the maximum number of nanoseconds by which a file
// timestamp may have shifted between stat calls. On Unix the granularity is
// typically 1 ns; on Windows it is 10000000 (10 ms). We use 2x granularity as
// a safety margin.
func racilyCleanWindow() int64 {
	racilyCleanStat.Do(func() {
		racilyCleanStat.granularity = 1
		if runtime.GOOS == "windows" {
			racilyCleanStat.granularity = 10_000_000
		}
	})
	return racilyCleanStat.granularity * 2
}

// mtimeAndSize is intentionally private. Filesystem stamps are an I/O detail
// and never cross the store/model boundary.
type mtimeAndSize struct {
	mtimeNsec int64
	size      int64
}

func statFile(path string) (mtimeAndSize, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return mtimeAndSize{}, err
	}
	return mtimeAndSize{mtimeNsec: info.ModTime().UnixNano(), size: info.Size()}, nil
}

// These seams are package-private so store tests can exercise the exact retry
// contract without exporting filesystem stamp types or changing production
// callers.
var (
	// racilyCleanNow remains a package-local compatibility seam for callers
	// that used to control wall-clock age; RacilyClean intentionally does not
	// consult it for metadata reuse.
	racilyCleanNow      = time.Now
	racilyCleanStatPath = statFile
)

func sameMtimeAndSize(left, right mtimeAndSize) bool {
	return left.mtimeNsec == right.mtimeNsec && left.size == right.size
}

// safelyReadAfterStoredMtime reports whether the persisted successful
// read/index timestamp is definitely later than the persisted file mtime.
// IndexedAt is stored as Unix seconds, so a same-second value is intentionally
// treated as too coarse and requires a body hash.
func safelyReadAfterStoredMtime(state model.DocArtifactState) bool {
	const maxUnixSecondsForNsec = int64(9223372036)
	if state.IndexedAt <= 0 || state.IndexedAt > maxUnixSecondsForNsec {
		return false
	}
	indexedAtNsec := state.IndexedAt * int64(time.Second)
	if indexedAtNsec <= state.MtimeNsec {
		return false
	}
	return indexedAtNsec-state.MtimeNsec > racilyCleanWindow()
}

func concurrentChangeError(absolutePath string, stored model.DocArtifactState, current mtimeAndSize, attempt int) error {
	path := strings.TrimSpace(stored.Path)
	if path == "" {
		path = absolutePath
	}
	return &model.ErrConcurrentChange{
		Path:             path,
		StoredMtimeNsec:  stored.MtimeNsec,
		StoredSize:       stored.Size,
		CurrentMtimeNsec: current.mtimeNsec,
		CurrentSize:      current.size,
		Attempt:          attempt,
	}
}

// RacilyClean compares a stored artifact state against the current filesystem
// state. A metadata match is safe to reuse without reading the body only when
// the stored successful read/index time is safely after the stored mtime.
// Missing or too-coarse index timestamps force a body hash so recent
// same-size/same-mtime rewrites are detected. Metadata changes use
// stat-before/read/stat-after and retry exactly once when the file is unstable;
// a second unstable read returns ErrConcurrentChange.
func RacilyClean(
	ctx context.Context,
	absolutePath string,
	currentRead func() ([]byte, error),
	storedState model.DocArtifactState,
) (currentHash string, reusesStored bool, err error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if strings.TrimSpace(absolutePath) == "" || !filepath.IsAbs(absolutePath) {
		return "", false, fmt.Errorf("racily-clean requires an absolute path")
	}
	if currentRead == nil {
		return "", false, fmt.Errorf("racily-clean requires a read function")
	}

	for attempt := 1; attempt <= 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		before, err := racilyCleanStatPath(absolutePath)
		if err != nil {
			return "", false, err
		}

		metadataEqual := before.mtimeNsec == storedState.MtimeNsec && before.size == storedState.Size
		if metadataEqual && storedState.ContentSHA256 != "" && safelyReadAfterStoredMtime(storedState) {
			return "", true, nil
		}

		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		content, readErr := currentRead()
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		if readErr != nil {
			return "", false, readErr
		}
		digest := sha256.Sum256(content)
		currentHash = hex.EncodeToString(digest[:])

		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		after, err := racilyCleanStatPath(absolutePath)
		if err != nil {
			return "", false, err
		}
		if sameMtimeAndSize(before, after) {
			if metadataEqual && currentHash == storedState.ContentSHA256 {
				return "", true, nil
			}
			return currentHash, false, nil
		}

		if attempt == 2 {
			return "", false, concurrentChangeError(absolutePath, storedState, after, attempt)
		}
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		// The next iteration obtains a fresh stat-before and performs one retry.
	}
	return "", false, concurrentChangeError(absolutePath, storedState, mtimeAndSize{}, 2)
}

// --- Artifact state queries ---

// UpsertDocArtifactState inserts or replaces the state row for a single path.
func UpsertDocArtifactState(ctx context.Context, db *sql.DB, state model.DocArtifactState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO doc_artifact_states(path, size, mtime_nsec, content_sha256,
			parser_version, authority_config_hash, indexed_at, last_docs_gen_at, lifecycle)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			size=excluded.size,
			mtime_nsec=excluded.mtime_nsec,
			content_sha256=excluded.content_sha256,
			parser_version=excluded.parser_version,
			authority_config_hash=excluded.authority_config_hash,
			indexed_at=excluded.indexed_at,
			last_docs_gen_at=excluded.last_docs_gen_at,
			lifecycle=excluded.lifecycle
	`, state.Path, state.Size, state.MtimeNsec, state.ContentSHA256,
		state.ParserVersion, state.AuthorityConfigHash,
		state.IndexedAt, state.LastDocsGenAt, state.Lifecycle)
	return err
}

// UpsertDocArtifactStateTx is the transaction-local variant.
func UpsertDocArtifactStateTx(ctx context.Context, tx *sql.Tx, state model.DocArtifactState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO doc_artifact_states(path, size, mtime_nsec, content_sha256,
			parser_version, authority_config_hash, indexed_at, last_docs_gen_at, lifecycle)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			size=excluded.size,
			mtime_nsec=excluded.mtime_nsec,
			content_sha256=excluded.content_sha256,
			parser_version=excluded.parser_version,
			authority_config_hash=excluded.authority_config_hash,
			indexed_at=excluded.indexed_at,
			last_docs_gen_at=excluded.last_docs_gen_at,
			lifecycle=excluded.lifecycle
	`, state.Path, state.Size, state.MtimeNsec, state.ContentSHA256,
		state.ParserVersion, state.AuthorityConfigHash,
		state.IndexedAt, state.LastDocsGenAt, state.Lifecycle)
	return err
}

// DeleteDocArtifactState removes an active or deprecated document state. A
// retired state is historical evidence and is deliberately preserved.
func DeleteDocArtifactState(ctx context.Context, db *sql.DB, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	state, ok, err := GetDocArtifactState(ctx, db, path)
	if err != nil || !ok || state.Lifecycle == model.DocLifecycleRetired {
		return err
	}
	_, err = db.ExecContext(ctx, "DELETE FROM doc_artifact_states WHERE path=?", path)
	return err
}

// DeleteDocArtifactStateTx is the transaction-local variant.
func DeleteDocArtifactStateTx(ctx context.Context, tx *sql.Tx, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	state, ok, err := GetDocArtifactStateTx(ctx, tx, path)
	if err != nil || !ok || state.Lifecycle == model.DocLifecycleRetired {
		return err
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM doc_artifact_states WHERE path=?", path)
	return err
}

// GetDocArtifactState reads the current state for a single path.
func GetDocArtifactState(ctx context.Context, db *sql.DB, path string) (model.DocArtifactState, bool, error) {
	var state model.DocArtifactState
	if err := ctx.Err(); err != nil {
		return state, false, err
	}
	err := db.QueryRowContext(ctx, `
		SELECT path, size, mtime_nsec, content_sha256, parser_version,
		       authority_config_hash, indexed_at, last_docs_gen_at, lifecycle
		FROM doc_artifact_states WHERE path=?
	`, path).Scan(
		&state.Path, &state.Size, &state.MtimeNsec, &state.ContentSHA256,
		&state.ParserVersion, &state.AuthorityConfigHash,
		&state.IndexedAt, &state.LastDocsGenAt, &state.Lifecycle,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return state, false, nil
		}
		return state, false, err
	}
	return state, true, nil
}

// GetDocArtifactStateTx is the transaction-local variant.
func GetDocArtifactStateTx(ctx context.Context, tx *sql.Tx, path string) (model.DocArtifactState, bool, error) {
	var state model.DocArtifactState
	if err := ctx.Err(); err != nil {
		return state, false, err
	}
	err := tx.QueryRowContext(ctx, `
		SELECT path, size, mtime_nsec, content_sha256, parser_version,
		       authority_config_hash, indexed_at, last_docs_gen_at, lifecycle
		FROM doc_artifact_states WHERE path=?
	`, path).Scan(
		&state.Path, &state.Size, &state.MtimeNsec, &state.ContentSHA256,
		&state.ParserVersion, &state.AuthorityConfigHash,
		&state.IndexedAt, &state.LastDocsGenAt, &state.Lifecycle,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return state, false, nil
		}
		return state, false, err
	}
	return state, true, nil
}

// ListDocArtifactStates returns all artifact states ordered by path.
func ListDocArtifactStates(ctx context.Context, db *sql.DB) ([]model.DocArtifactState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := QueryContextWithRetry(ctx, db, `
		SELECT path, size, mtime_nsec, content_sha256, parser_version,
		       authority_config_hash, indexed_at, last_docs_gen_at, lifecycle
		FROM doc_artifact_states ORDER BY path ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.DocArtifactState, 0)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var state model.DocArtifactState
		if err := rows.Scan(&state.Path, &state.Size, &state.MtimeNsec, &state.ContentSHA256,
			&state.ParserVersion, &state.AuthorityConfigHash, &state.IndexedAt, &state.LastDocsGenAt, &state.Lifecycle); err != nil {
			return nil, err
		}
		items = append(items, state)
	}
	return items, rows.Err()
}

// ListDocArtifactStatesForLifecycle returns states matching a lifecycle filter.
func ListDocArtifactStatesForLifecycle(ctx context.Context, db *sql.DB, lifecycle string) ([]model.DocArtifactState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := QueryContextWithRetry(ctx, db, `
		SELECT path, size, mtime_nsec, content_sha256, parser_version,
		       authority_config_hash, indexed_at, last_docs_gen_at, lifecycle
		FROM doc_artifact_states WHERE lifecycle=? ORDER BY path ASC
	`, lifecycle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.DocArtifactState, 0)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var state model.DocArtifactState
		if err := rows.Scan(&state.Path, &state.Size, &state.MtimeNsec, &state.ContentSHA256,
			&state.ParserVersion, &state.AuthorityConfigHash, &state.IndexedAt, &state.LastDocsGenAt, &state.Lifecycle); err != nil {
			return nil, err
		}
		items = append(items, state)
	}
	return items, rows.Err()
}

// --- Deletion proof helpers ---

// IsDiskAbsent returns true only for a proven ENOENT condition. Permission and
// other filesystem errors are not positive deletion proof.
func IsDiskAbsent(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, os.ErrNotExist)
}

// NormalizeRepoRelative converts an absolute or workspace-absolute path to a
// repo-relative POSIX path suitable as the doc_artifact_states primary key.
func NormalizeRepoRelative(workspaceRoot, absPath string) string {
	rel, err := filepath.Rel(workspaceRoot, absPath)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return ""
	}
	return rel
}
