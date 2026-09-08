package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// replaceFileSymbols deletes and re-inserts symbols and file record for a single file.
// This is used by the file watcher for incremental indexing.
func replaceFileSymbols(ctx context.Context, db *sql.DB, filePath string, repoID string, repoName string, language string, contentHash string, symbols []model.SymbolRecord) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := replaceFileSymbolsTx(ctx, tx, filePath, repoID, repoName, language, contentHash, symbols); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceFileSymbolsTx(ctx context.Context, tx *sql.Tx, filePath string, repoID string, repoName string, language string, contentHash string, symbols []model.SymbolRecord) error {
	// Delete existing symbols and file record for this path.
	if _, err := tx.ExecContext(ctx, "DELETE FROM symbols WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE file_path = ?", filePath); err != nil {
		return err
	}

	// Insert updated file record.
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO files(file_path, repo_id, repo_name, content_hash, indexed_at, language) VALUES(?, ?, ?, ?, ?, ?)",
		filePath, repoID, repoName, contentHash, time.Now().Unix(), language,
	); err != nil {
		return err
	}

	// Insert updated symbols.
	if len(symbols) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO symbols(
			file_path, repo_id, repo_name, name, kind, start_line, end_line, parent, qualified_name, signature, signature_hash, scope, language, file_hash, implements, search_text
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sym := range symbols {
		if _, err := stmt.ExecContext(ctx,
			sym.FilePath, sym.RepoID, sym.RepoName, sym.Name, sym.Kind,
			sym.StartLine, sym.EndLine, sym.Parent, sym.QualifiedName,
			sym.Signature, sym.SignatureHash, sym.Scope, sym.Language,
			sym.FileHash, sym.Implements, sym.SearchText,
		); err != nil {
			return err
		}
	}
	return nil
}

// deleteFileSymbols removes all symbols and file record for a deleted file.
// Used by incremental indexing when files are deleted.
func deleteFileSymbols(ctx context.Context, db *sql.DB, filePath string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteFileSymbolsTx(ctx, tx, filePath); err != nil {
		return err
	}
	return tx.Commit()
}

func deleteFileSymbolsTx(ctx context.Context, tx *sql.Tx, filePath string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM symbols WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE file_path = ?", filePath); err != nil {
		return err
	}
	return nil
}

// ========================
// Incremental Document Changes (T4)
// ========================

// IncrementalDocChange represents one document change discovered during a
// reconciler pass. Action is "replace" for new/changed content and "delete"
// for proven deletions. Complete parsed rows are carried for replace;
// prior path identity and proof classification for delete.
type IncrementalDocChange struct {
	Action   string // "replace" | "delete"
	Path     string
	Proof    string // "disk-absent" | "scope-changed"
	Content  []byte // stable read used to build the state and parse the document
	Doc      *model.DocRecord
	Edges    []model.DocEdge
	Mentions []model.DocMention
	Blocks   []model.DocSourceBlock
	Records  []model.DocSourceRecord
	Bindings []model.DocArtifactBinding
	State    *model.DocArtifactState
}

// ErrUnexplainedShrink is returned when rows outside the explicit change set
// are modified by an incremental document transaction.
var ErrUnexplainedShrink = &shrinkError{"unexplained row loss detected; unrelated document rows changed during incremental publication"}

type shrinkError struct{ reason string }

func (e *shrinkError) Error() string { return e.reason }

type docOwnerTable struct {
	table string
	owner string
}

var incrementalDocOwnerTables = []docOwnerTable{
	{table: "doc_artifact_bindings", owner: "doc_path"},
	{table: "doc_source_records", owner: "doc_path"},
	{table: "doc_source_blocks", owner: "doc_path"},
	{table: "doc_mentions", owner: "doc_path"},
	{table: "doc_edges", owner: "from_path"},
	{table: "doc_records", owner: "path"},
	{table: "doc_artifact_states", owner: "path"},
}

func validateIncrementalDocChanges(changes []IncrementalDocChange) (map[string]struct{}, error) {
	if len(changes) == 0 {
		return nil, fmt.Errorf("incremental document publication requires at least one change")
	}
	owned := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		path := strings.TrimSpace(change.Path)
		if path == "" {
			return nil, fmt.Errorf("incremental document change has an empty path")
		}
		if change.Path != path {
			return nil, fmt.Errorf("incremental document path %q is not normalized", change.Path)
		}
		if _, exists := owned[path]; exists {
			return nil, fmt.Errorf("duplicate incremental document change for %s", path)
		}
		owned[path] = struct{}{}
		if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("incremental document path %q must be repo-relative", path)
		}
		switch change.Action {
		case "replace":
			if change.Doc == nil {
				return nil, fmt.Errorf("replace change for %s has no parsed document", path)
			}
			if strings.TrimSpace(change.Doc.Path) != path {
				return nil, fmt.Errorf("replace document path %q does not match owner %q", change.Doc.Path, path)
			}
			if change.State == nil || strings.TrimSpace(change.State.ContentSHA256) == "" {
				return nil, fmt.Errorf("replace change for %s is missing a complete SHA-256 state", path)
			}
			if change.State.Path != "" && change.State.Path != path {
				return nil, fmt.Errorf("replace state path %q does not match owner %q", change.State.Path, path)
			}
		case "delete":
			switch change.Proof {
			case "disk-absent", "scope-changed":
			default:
				return nil, fmt.Errorf("delete change for %s lacks positive deletion proof", path)
			}
		default:
			return nil, fmt.Errorf("unknown incremental document action %q for %s", change.Action, path)
		}
	}
	return owned, nil
}

// deleteDocOwnedRowsTx deletes all rows owned by a single doc path. Each table
// has an explicit owner column: doc_records uses path, while doc_edges uses
// from_path. Keeping this mapping explicit prevents no-such-column failures
// and prevents an accidental table-wide delete.
func deleteDocOwnedRowsTx(ctx context.Context, tx *sql.Tx, docPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, ownerTable := range incrementalDocOwnerTables {
		if ownerTable.table == "doc_artifact_states" {
			continue
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+ownerTable.table+" WHERE "+ownerTable.owner+" = ?", docPath); err != nil {
			return err
		}
	}
	return nil
}

func insertDocReplaceTx(ctx context.Context, tx *sql.Tx, change IncrementalDocChange) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := change.Path
	if change.Doc == nil || strings.TrimSpace(change.Doc.Path) != path {
		return fmt.Errorf("replace change for %s is missing a matching document", path)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO doc_records(path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at, is_snapshot)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, change.Doc.Path, change.Doc.Title, change.Doc.DocID, change.Doc.Layer, change.Doc.Family,
		change.Doc.Snippet, change.Doc.SearchText, change.Doc.ContentHash, change.Doc.IndexedAt, change.Doc.IsSnapshot); err != nil {
		return err
	}

	for _, edge := range change.Edges {
		if edge.FromPath != path {
			return fmt.Errorf("edge owner %q does not match document %q", edge.FromPath, path)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO doc_edges(from_path, to_path, to_doc_id, kind, label)
			VALUES(?, ?, ?, ?, ?)
		`, edge.FromPath, edge.ToPath, edge.ToDocID, edge.Kind, edge.Label); err != nil {
			return err
		}
	}

	for _, mention := range change.Mentions {
		if mention.DocPath != path {
			return fmt.Errorf("mention owner %q does not match document %q", mention.DocPath, path)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO doc_mentions(doc_path, mention_type, mention_value, source_block)
			VALUES(?, ?, ?, ?)
		`, mention.DocPath, mention.MentionType, mention.MentionValue, nullableDocMentionSourceBlock(mention.SourceBlock)); err != nil {
			return err
		}
	}

	for _, block := range change.Blocks {
		if block.DocPath != path {
			return fmt.Errorf("source block owner %q does not match document %q", block.DocPath, path)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO doc_source_blocks(doc_path, block_id, doc_id, kind, source_format, ordinal, start_line, end_line, content_hash, indexed_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, block.DocPath, block.BlockID, block.DocID, block.Kind, block.SourceFormat, block.Ordinal,
			block.StartLine, block.EndLine, block.ContentHash, block.IndexedAt); err != nil {
			return err
		}
	}

	for _, record := range change.Records {
		if record.DocPath != path {
			return fmt.Errorf("source record owner %q does not match document %q", record.DocPath, path)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO doc_source_records(doc_path, block_id, record_id, record_type, ordinal, start_line, end_line, content_hash, indexed_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, record.DocPath, record.BlockID, record.RecordID, record.RecordType, record.Ordinal,
			record.StartLine, record.EndLine, record.ContentHash, record.IndexedAt); err != nil {
			return err
		}
	}

	for _, binding := range change.Bindings {
		if binding.DocPath != path {
			return fmt.Errorf("binding owner %q does not match document %q", binding.DocPath, path)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO doc_artifact_bindings(doc_path, block_id, doc_id, relation, role, target_path, target_symbol, target_kind, authoring_origin, binding_status, doc_lifecycle, superseded_by, ordinal, start_line, end_line, source_content_hash, binding_ref, indexed_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, binding.DocPath, binding.BlockID, binding.DocID, binding.Relation, binding.Role,
			binding.TargetPath, binding.TargetSymbol, binding.TargetKind, binding.AuthoringOrigin,
			binding.BindingStatus, binding.DocLifecycle, binding.SupersededBy, binding.Ordinal,
			binding.StartLine, binding.EndLine, binding.SourceContentHash, binding.BindingRef, binding.IndexedAt); err != nil {
			return err
		}
	}
	return nil
}

func deleteDocDeleteTx(ctx context.Context, tx *sql.Tx, docPath string) error {
	return deleteDocOwnedRowsTx(ctx, tx, docPath)
}

type docRowsFingerprint struct {
	count  int
	digest string
}

func fingerprintDocRowsTx(ctx context.Context, tx *sql.Tx, ownerPaths map[string]struct{}) (map[string]docRowsFingerprint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(ownerPaths))
	for path := range ownerPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make(map[string]docRowsFingerprint, len(incrementalDocOwnerTables))
	for _, ownerTable := range incrementalDocOwnerTables {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		query := "SELECT * FROM " + ownerTable.table
		args := make([]any, 0, len(paths))
		if len(paths) > 0 {
			placeholders := make([]string, len(paths))
			for i, path := range paths {
				placeholders[i] = "?"
				args = append(args, path)
			}
			query += " WHERE " + ownerTable.owner + " NOT IN (" + strings.Join(placeholders, ",") + ")"
		}
		query += " ORDER BY rowid"
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		h := sha256.New()
		_, _ = h.Write([]byte(strings.Join(columns, "\x00")))
		count := 0
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				_ = rows.Close()
				return nil, err
			}
			count++
			for _, value := range values {
				switch typed := value.(type) {
				case nil:
					_, _ = fmt.Fprint(h, "<nil>;")
				case []byte:
					_, _ = fmt.Fprintf(h, "bytes:%x;", typed)
				default:
					_, _ = fmt.Fprintf(h, "%T:%v;", value, value)
				}
			}
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if rowsErr != nil {
			return nil, rowsErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		result[ownerTable.table] = docRowsFingerprint{count: count, digest: hex.EncodeToString(h.Sum(nil))}
	}
	return result, nil
}

func ensureUnrelatedDocRowsStable(before, after map[string]docRowsFingerprint) error {
	for table, prior := range before {
		current, ok := after[table]
		if !ok || prior != current {
			return ErrUnexplainedShrink
		}
	}
	return nil
}

// shrinkGuard validates a change set before it is applied. The actual before /
// after invariant is enforced by applyIncrementalDocChangesTx; this wrapper is
// kept as a fail-closed compatibility seam for package-local callers.
func shrinkGuard(ctx context.Context, tx *sql.Tx, changes []IncrementalDocChange) error {
	_, err := validateIncrementalDocChanges(changes)
	return err
}

func docPathHasRetiredLifecycleTx(ctx context.Context, tx *sql.Tx, path string) (bool, error) {
	state, ok, err := GetDocArtifactStateTx(ctx, tx, path)
	if err != nil {
		return false, err
	}
	if ok {
		return state.Lifecycle == model.DocLifecycleRetired, nil
	}
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM doc_artifact_bindings WHERE doc_path=? AND doc_lifecycle=? LIMIT 1`, path, model.DocLifecycleRetired).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func docPathHasOwnedRowsTx(ctx context.Context, tx *sql.Tx, path string) (bool, error) {
	for _, ownerTable := range incrementalDocOwnerTables {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		var exists int
		err := tx.QueryRowContext(ctx, "SELECT 1 FROM "+ownerTable.table+" WHERE "+ownerTable.owner+"=? LIMIT 1", path).Scan(&exists)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
	}
	return false, nil
}

// applyIncrementalDocChangesTx applies only explicit owner-path changes and
// returns whether at least one real replace/delete mutation occurred. It is
// shared by foreground and fenced job publication so both paths have the same
// row ownership and retired-history semantics.
func applyIncrementalDocChangesTx(ctx context.Context, tx *sql.Tx, changes []IncrementalDocChange) (bool, error) {
	if err := shrinkGuard(ctx, tx, changes); err != nil {
		return false, err
	}
	ownedPaths, err := validateIncrementalDocChanges(changes)
	if err != nil {
		return false, err
	}
	before, err := fingerprintDocRowsTx(ctx, tx, ownedPaths)
	if err != nil {
		return false, err
	}
	actual := false
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		switch change.Action {
		case "replace":
			if err := deleteDocOwnedRowsTx(ctx, tx, change.Path); err != nil {
				return false, err
			}
			if err := insertDocReplaceTx(ctx, tx, change); err != nil {
				return false, fmt.Errorf("insert replace for %s: %w", change.Path, err)
			}
			if change.State != nil {
				state := *change.State
				if state.Path == "" {
					state.Path = change.Path
				}
				if state.Lifecycle == "" {
					state.Lifecycle = model.DocLifecycleActive
				}
				if err := UpsertDocArtifactStateTx(ctx, tx, state); err != nil {
					return false, err
				}
			} else if err := DeleteDocArtifactStateTx(ctx, tx, change.Path); err != nil {
				// A legacy caller without a state must not leave an obsolete
				// active state behind; retired history remains protected.
				return false, err
			}
			actual = true
		case "delete":
			retired, err := docPathHasRetiredLifecycleTx(ctx, tx, change.Path)
			if err != nil {
				return false, err
			}
			if retired {
				continue
			}
			exists, err := docPathHasOwnedRowsTx(ctx, tx, change.Path)
			if err != nil {
				return false, err
			}
			if !exists {
				continue
			}
			if err := deleteDocDeleteTx(ctx, tx, change.Path); err != nil {
				return false, err
			}
			if err := DeleteDocArtifactStateTx(ctx, tx, change.Path); err != nil {
				return false, err
			}
			actual = true
		}
	}
	after, err := fingerprintDocRowsTx(ctx, tx, ownedPaths)
	if err != nil {
		return false, err
	}
	if err := ensureUnrelatedDocRowsStable(before, after); err != nil {
		return false, err
	}
	if actual {
		if err := invalidateDocIdentitySnapshotTx(ctx, tx); err != nil {
			return false, err
		}
	}
	return actual, nil
}
