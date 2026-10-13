package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type docBindingSourceQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const docBindingSourceColumns = `s.doc_path, s.binding_id, s.doc_id, s.block_id, s.kind, s.role, s.path, s.surface, s.binding_lifecycle, s.symbol, s.document_lifecycle, s.provenance, s.binding_ref, s.source_content_hash, s.start_line, s.end_line`

func incompleteDocBindingSources(reason string) model.DocBindingSourceCollection {
	return model.DocBindingSourceCollection{Version: model.DocBindingSourceVersion, Availability: model.DocBindingSourceIncomplete, Reason: reason, Declarations: []model.DocBindingSource{}}
}

// docBindingSourceTablesPresent never creates or migrates anything. Schema
// absence is distinct from unexpected database/query errors.
func docBindingSourceTablesPresent(ctx context.Context, q docBindingSourceQueryer) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('doc_binding_sources','doc_binding_source_documents')`).Scan(&count)
	return count == 2, err
}

// ReadDocBindingSources is the bounded internal source lookup for Service.
// Selectors are exact and combined with AND. Either selector must be nonempty;
// a naked binding ID returns all matching owners, never a scored winner.
// All lifecycles are returned; the caller owns explicit historical selection.
// This function is read-only, including on pre-collection databases.
func ReadDocBindingSources(ctx context.Context, db *sql.DB, docPath, bindingID string, limit int) (model.DocBindingSourceCollection, error) {
	if db == nil {
		return incompleteDocBindingSources("database_unavailable"), sql.ErrConnDone
	}
	if docPath == "" && bindingID == "" {
		return incompleteDocBindingSources("selector_required"), fmt.Errorf("source lookup requires doc_path and/or binding_id")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return incompleteDocBindingSources("database_unavailable"), err
	}
	defer func() { _ = tx.Rollback() }()
	present, err := docBindingSourceTablesPresent(ctx, tx)
	if err != nil {
		return incompleteDocBindingSources("database_unavailable"), err
	}
	if !present {
		return incompleteDocBindingSources("canonical_source_collection_not_persisted"), nil
	}
	out := model.DocBindingSourceCollection{Version: model.DocBindingSourceVersion, Availability: model.DocBindingSourceAvailable, Declarations: []model.DocBindingSource{}}
	if docPath != "" {
		var version string
		err := tx.QueryRowContext(ctx, `SELECT collection_version, document_lifecycle FROM doc_binding_source_documents WHERE doc_path=?`, docPath).Scan(&version, &out.DocumentLifecycle)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && version != model.DocBindingSourceVersion) {
			out = incompleteDocBindingSources("source_owner_not_published")
		} else if err != nil {
			return out, err
		}
	} else {
		// A global-ID lookup cannot claim completeness when any document was
		// never reparsed by the source producer. It may still return known rows.
		var missing int
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM doc_records d LEFT JOIN doc_binding_source_documents o ON o.doc_path=d.path AND o.collection_version=? WHERE o.doc_path IS NULL)`, model.DocBindingSourceVersion).Scan(&missing)
		if err != nil {
			return out, err
		}
		if missing != 0 {
			out = incompleteDocBindingSources("source_owners_not_published")
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+docBindingSourceColumns+` FROM doc_binding_sources s JOIN doc_binding_source_documents o ON o.doc_path=s.doc_path AND o.collection_version=? WHERE (?='' OR s.doc_path=?) AND (?='' OR s.binding_id=?) ORDER BY s.doc_path, s.binding_id LIMIT ?`, model.DocBindingSourceVersion, docPath, docPath, bindingID, bindingID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanDocBindingSource(rows)
		if err != nil {
			return out, err
		}
		if len(out.Declarations) == limit {
			out.Truncated = true
			break
		}
		out.Declarations = append(out.Declarations, item)
	}
	return out, rows.Err()
}

func scanDocBindingSource(rows *sql.Rows) (model.DocBindingSource, error) {
	var item model.DocBindingSource
	err := rows.Scan(&item.DocPath, &item.Binding.BindingID, &item.DocID, &item.BlockID, &item.Binding.Kind, &item.Binding.Role, &item.Binding.Path, &item.Binding.Surface, &item.Binding.BindingLifecycle, &item.Binding.Symbol, &item.DocumentLifecycle, &item.Provenance, &item.BindingRef, &item.SourceContentHash, &item.StartLine, &item.EndLine)
	return item, err
}

// hydrateDocBindingSourcesTx transports all producer rows in the SAME read
// snapshot as docs/bridge/identity marker. This is internal snapshot transport,
// not a public unbounded query. Missing tables remain explicit omissions.
func hydrateDocBindingSourcesTx(ctx context.Context, tx *sql.Tx, docs []model.DocRecord) error {
	byPath := make(map[string]*model.DocBindingSourceCollection, len(docs))
	for i := range docs {
		collection := incompleteDocBindingSources("canonical_source_collection_not_persisted")
		docs[i].BindingSources = &collection
		byPath[docs[i].Path] = &collection
	}
	present, err := docBindingSourceTablesPresent(ctx, tx)
	if err != nil || !present {
		return err
	}
	owners, err := tx.QueryContext(ctx, `SELECT doc_path, collection_version, document_lifecycle FROM doc_binding_source_documents ORDER BY doc_path`)
	if err != nil {
		return err
	}
	for owners.Next() {
		var path, version, lifecycle string
		if err := owners.Scan(&path, &version, &lifecycle); err != nil {
			_ = owners.Close()
			return err
		}
		if collection := byPath[path]; collection != nil && version == model.DocBindingSourceVersion {
			collection.Availability = model.DocBindingSourceAvailable
			collection.Reason = ""
			collection.DocumentLifecycle = lifecycle
		}
	}
	if err := owners.Err(); err != nil {
		_ = owners.Close()
		return err
	}
	if err := owners.Close(); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+docBindingSourceColumns+` FROM doc_binding_sources s ORDER BY s.doc_path, s.binding_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanDocBindingSource(rows)
		if err != nil {
			return err
		}
		if collection := byPath[item.DocPath]; collection != nil && collection.Availability == model.DocBindingSourceAvailable {
			collection.Declarations = append(collection.Declarations, item)
		}
	}
	return rows.Err()
}

func insertDocBindingSourcesTx(ctx context.Context, tx *sql.Tx, doc model.DocRecord) error {
	collection := doc.BindingSources
	if collection == nil || collection.Availability != model.DocBindingSourceAvailable {
		return nil // legacy caller cannot mint a parsed-empty source collection
	}
	if collection.Version != model.DocBindingSourceVersion || collection.Truncated {
		return fmt.Errorf("incomplete or unsupported source collection for %s", doc.Path)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO doc_binding_source_documents(doc_path, collection_version, document_lifecycle) VALUES(?,?,?)`, doc.Path, collection.Version, collection.DocumentLifecycle); err != nil {
		return err
	}
	for _, item := range collection.Declarations {
		if item.DocPath != doc.Path || item.DocID != doc.DocID || item.Binding.BindingID == "" || item.Provenance != "declared" || item.DocumentLifecycle != collection.DocumentLifecycle {
			return fmt.Errorf("source declaration owner/identity mismatch for %s", doc.Path)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO doc_binding_sources(doc_path, binding_id, doc_id, block_id, kind, role, path, surface, binding_lifecycle, symbol, document_lifecycle, provenance, binding_ref, source_content_hash, start_line, end_line) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.DocPath, item.Binding.BindingID, item.DocID, item.BlockID, item.Binding.Kind, item.Binding.Role, item.Binding.Path, item.Binding.Surface, item.Binding.BindingLifecycle, item.Binding.Symbol, item.DocumentLifecycle, item.Provenance, item.BindingRef, item.SourceContentHash, item.StartLine, item.EndLine); err != nil {
			return err
		}
	}
	return nil
}

// Full publication keeps retired owners omitted by a scan. Active excluded
// owners are transported by Docgraph's prior snapshot retention (Store does
// not inspect disk or infer scope). The incoming owners remain replacements.
func retainedRetiredDocOwnersTx(ctx context.Context, tx *sql.Tx, docs []model.DocRecord) ([]string, error) {
	incoming := make(map[string]bool, len(docs))
	for _, doc := range docs {
		incoming[doc.Path] = true
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.path FROM doc_records d WHERE EXISTS(SELECT 1 FROM doc_artifact_states a WHERE a.path=d.path AND a.lifecycle='retired') OR EXISTS(SELECT 1 FROM doc_binding_source_documents o WHERE o.doc_path=d.path AND o.document_lifecycle='retired') OR EXISTS(SELECT 1 FROM doc_artifact_bindings b WHERE b.doc_path=d.path AND b.doc_lifecycle='retired') ORDER BY d.path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var retained []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if !incoming[path] {
			retained = append(retained, path)
		}
	}
	return retained, rows.Err()
}

// Reject a terminal retired owner becoming active through a producer rewrite.
// This also protects binding-free retired documents via the owner table.
func guardRetiredSourceReplacementTx(ctx context.Context, tx *sql.Tx, doc model.DocRecord) error {
	retired, err := docPathHasRetiredLifecycleTx(ctx, tx, doc.Path)
	if err != nil {
		return err
	}
	if retired && (doc.BindingSources == nil || doc.BindingSources.DocumentLifecycle != model.DocLifecycleRetired) {
		return fmt.Errorf("retired source owner %s cannot be reactivated or replaced without its historical lifecycle", doc.Path)
	}
	if doc.BindingSources != nil && doc.BindingSources.DocumentLifecycle != model.DocLifecycleRetired && doc.DocID != "" {
		var terminal int
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM doc_records d JOIN doc_binding_source_documents o ON o.doc_path=d.path WHERE d.doc_id=? AND o.document_lifecycle='retired')`, doc.DocID).Scan(&terminal)
		if err != nil {
			return err
		}
		if terminal != 0 {
			return fmt.Errorf("retired document ID %s cannot be reused", doc.DocID)
		}
	}
	return nil
}
