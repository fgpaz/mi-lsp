package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type readonlySnapshotEntry struct {
	Mode   os.FileMode
	Size   int64
	Mtime  int64
	SHA256 string
	IsDir  bool
}

func TestOpenReadOnlyExistingAbsentDoesNotCreateState(t *testing.T) {
	root := t.TempDir()
	before := snapshotFiles(t, root)
	if _, err := OpenReadOnlyExisting(root, WorkspaceDBPath(root)); err == nil {
		t.Fatal("missing database should not open")
	}
	after := snapshotFiles(t, root)
	if len(before) != len(after) {
		t.Fatalf("filesystem changed: before=%v after=%v", before, after)
	}
}

func TestOpenReadOnlyExistingUsesRealReadOnlyMode(t *testing.T) {
	root := t.TempDir()
	db, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.WriteFile(WorkspaceDBPath(root)+"-wal", []byte("preexisting wal"), 0o644); err != nil {
		t.Fatalf("write preexisting WAL: %v", err)
	}
	if err := os.WriteFile(WorkspaceDBPath(root)+"-shm", []byte("preexisting shm"), 0o644); err != nil {
		t.Fatalf("write preexisting SHM: %v", err)
	}
	before := snapshotFiles(t, root)
	ro, err := OpenReadOnlyExisting(root, WorkspaceDBPath(root))
	if err != nil {
		t.Fatalf("OpenReadOnlyExisting: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ro.PingContext(ctx); err != nil {
		t.Fatalf("PingContext: %v", err)
	}
	if _, err := ro.ExecContext(ctx, "CREATE TABLE probe_forbidden(id INTEGER)"); err == nil {
		t.Fatal("read-only connection accepted a write")
	}
	if err := ro.Close(); err != nil {
		t.Fatalf("Close read-only connection: %v", err)
	}
	after := snapshotFiles(t, root)
	assertSnapshotEqual(t, before, after)
}

func snapshotFiles(t *testing.T, root string) map[string]readonlySnapshotEntry {
	t.Helper()
	result := map[string]readonlySnapshotEntry{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		entry := readonlySnapshotEntry{
			Mode:  info.Mode(),
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
			IsDir: info.IsDir(),
		}
		if !info.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			digest := sha256.Sum256(data)
			entry.SHA256 = hex.EncodeToString(digest[:])
		}
		result[filepath.ToSlash(path)] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return result
}

func assertSnapshotEqual(t *testing.T, before, after map[string]readonlySnapshotEntry) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("snapshot length changed: before=%d after=%d", len(before), len(after))
	}
	paths := make([]string, 0, len(before))
	for path := range before {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		left, ok := before[path]
		right, exists := after[path]
		if !ok || !exists || left != right {
			t.Fatalf("snapshot changed at %q: before=%#v after=%#v", path, left, right)
		}
	}
}

type readOnlyQueryFixture struct {
	root    string
	docPath string
}

func seedReadOnlyQueryFixture(t *testing.T, legacy bool) readOnlyQueryFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	db, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	docPath := "docs/RF-READONLY.md"
	doc := model.DocRecord{
		Path:        docPath,
		Title:       "Read-only compatibility",
		DocID:       "RF-READONLY",
		Layer:       "04",
		Family:      "functional",
		SearchText:  "read-only compatibility",
		ContentHash: "doc-hash",
		IndexedAt:   1,
	}
	mention := model.DocMention{
		DocPath:      docPath,
		MentionType:  "test_file",
		MentionValue: "TP-READONLY-001",
		SourceBlock:  "frontmatter",
	}
	if err := ReplaceDocsWithSources(ctx, db, []model.DocRecord{doc}, nil, []model.DocMention{mention}, nil, nil, nil); err != nil {
		_ = db.Close()
		t.Fatalf("ReplaceDocsWithSources: %v", err)
	}

	bundle := testGraphBundle(t)
	bundle.Unresolved = []model.GraphUnresolved{{
		GenerationID:     bundle.Generation.GenerationID,
		UnresolvedID:     0,
		OwnerPath:        "docs/RF-READONLY.md",
		SubjectKind:      "document",
		SelectorDigest:   model.GraphDigest{7},
		ReasonCode:       "missing_doc_target",
		Candidates:       []string{"TP-READONLY-001"},
		Backend:          "docgraph",
		RecoveryHintCode: "inspect_doc_graph_reference",
		SourceDocument:   docPath,
		SourceBlock:      "frontmatter",
		TargetKind:       "document",
		TargetValue:      "TP-READONLY-001",
	}}
	bundle.Generation.UnresolvedCount = len(bundle.Unresolved)
	for i := range bundle.Unresolved {
		bundle.Unresolved[i].GenerationID = bundle.Generation.GenerationID
		bundle.Unresolved[i].UnresolvedKey = model.GraphUnresolvedKey(bundle.Unresolved[i])
		bundle.Unresolved[i].CrossRID = model.UnresolvedRID(bundle.Unresolved[i].UnresolvedKey)
	}
	if err := bundle.SealIDs(); err != nil {
		_ = db.Close()
		t.Fatalf("SealIDs: %v", err)
	}
	if err := StageGraphGeneration(ctx, db, &bundle); err != nil {
		_ = db.Close()
		t.Fatalf("StageGraphGeneration: %v", err)
	}
	if err := ActivateGraphGeneration(ctx, db, bundle.Generation.GenerationID, nil); err != nil {
		_ = db.Close()
		t.Fatalf("ActivateGraphGeneration: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	raw, err := sql.Open(driverName, WorkspaceDBPath(root))
	if err != nil {
		t.Fatalf("sql.Open fixture: %v", err)
	}
	if legacy {
		for _, stmt := range []string{
			"ALTER TABLE graph_unresolved DROP COLUMN source_document",
			"ALTER TABLE graph_unresolved DROP COLUMN source_block",
			"ALTER TABLE graph_unresolved DROP COLUMN target_kind",
			"ALTER TABLE graph_unresolved DROP COLUMN target_value",
			"ALTER TABLE doc_mentions DROP COLUMN source_block",
		} {
			if _, err := raw.ExecContext(ctx, stmt); err != nil {
				_ = raw.Close()
				t.Fatalf("legacy schema change %q: %v", stmt, err)
			}
		}
	}
	if _, err := raw.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = raw.Close()
		t.Fatalf("checkpoint fixture database: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close fixture schema connection: %v", err)
	}
	return readOnlyQueryFixture{root: root, docPath: docPath}
}

func TestOpenReadOnlyExistingReadsLegacyGraphAndDocsWithoutMutation(t *testing.T) {
	ctx := context.Background()
	fixture := seedReadOnlyQueryFixture(t, true)
	before := snapshotFiles(t, fixture.root)
	ro, err := OpenReadOnlyExisting(fixture.root, WorkspaceDBPath(fixture.root))
	if err != nil {
		t.Fatalf("OpenReadOnlyExisting: %v", err)
	}
	defer ro.Close()

	mentions, err := ListDocMentions(ctx, ro)
	if err != nil {
		t.Fatalf("ListDocMentions legacy: %v", err)
	}
	if len(mentions) != 1 || mentions[0].MentionValue != "TP-READONLY-001" || mentions[0].SourceBlock != "" {
		t.Fatalf("legacy mentions = %#v", mentions)
	}
	mentions, err = DocMentionsForPath(ctx, ro, fixture.docPath)
	if err != nil {
		t.Fatalf("DocMentionsForPath legacy: %v", err)
	}
	if len(mentions) != 1 || mentions[0].MentionValue != "TP-READONLY-001" || mentions[0].SourceBlock != "" {
		t.Fatalf("legacy path mentions = %#v", mentions)
	}

	snapshot, err := BeginGraphQuerySnapshot(ctx, ro, "")
	if err != nil {
		t.Fatalf("BeginGraphQuerySnapshot legacy: %v", err)
	}
	omissions, err := snapshot.UnresolvedOmissions(ctx, 10)
	_ = snapshot.Close()
	if err != nil {
		t.Fatalf("UnresolvedOmissions legacy: %v", err)
	}
	if len(omissions) != 1 {
		t.Fatalf("legacy omissions = %#v", omissions)
	}
	omission := omissions[0]
	if omission.Input == "" || omission.OwnerPath != "docs/RF-READONLY.md" ||
		omission.Reason != "missing_doc_target" || omission.ErrorCode != "inspect_doc_graph_reference" ||
		omission.SourceDocument != "" || omission.SourceBlock != "" ||
		omission.TargetKind != "" || omission.TargetValue != "" {
		t.Fatalf("legacy omission projection = %#v", omission)
	}
	if err := ro.Close(); err != nil {
		t.Fatalf("close legacy read-only database: %v", err)
	}
	after := snapshotFiles(t, fixture.root)
	assertSnapshotEqual(t, before, after)
}

func TestOpenReadOnlyExistingRetainsCurrentGraphAndDocsEnrichment(t *testing.T) {
	ctx := context.Background()
	fixture := seedReadOnlyQueryFixture(t, false)
	before := snapshotFiles(t, fixture.root)
	ro, err := OpenReadOnlyExisting(fixture.root, WorkspaceDBPath(fixture.root))
	if err != nil {
		t.Fatalf("OpenReadOnlyExisting: %v", err)
	}

	mentions, err := ListDocMentions(ctx, ro)
	if err != nil {
		t.Fatalf("ListDocMentions current schema: %v", err)
	}
	if len(mentions) != 1 || mentions[0].SourceBlock != "frontmatter" {
		t.Fatalf("current mentions = %#v", mentions)
	}
	snapshot, err := BeginGraphQuerySnapshot(ctx, ro, "")
	if err != nil {
		_ = ro.Close()
		t.Fatalf("BeginGraphQuerySnapshot current schema: %v", err)
	}
	omissions, err := snapshot.UnresolvedOmissions(ctx, 10)
	_ = snapshot.Close()
	if err != nil {
		_ = ro.Close()
		t.Fatalf("UnresolvedOmissions current schema: %v", err)
	}
	if len(omissions) != 1 || omissions[0].SourceDocument != fixture.docPath ||
		omissions[0].SourceBlock != "frontmatter" || omissions[0].TargetKind != "document" ||
		omissions[0].TargetValue != "TP-READONLY-001" {
		_ = ro.Close()
		t.Fatalf("current omission projection = %#v", omissions)
	}
	if err := ro.Close(); err != nil {
		t.Fatalf("close current read-only database: %v", err)
	}
	after := snapshotFiles(t, fixture.root)
	assertSnapshotEqual(t, before, after)
}

func TestOpenReadOnlyExistingDoesNotFallbackForNonCapabilityErrors(t *testing.T) {
	ctx := context.Background()
	fixture := seedReadOnlyQueryFixture(t, false)
	raw, err := sql.Open(driverName, WorkspaceDBPath(fixture.root))
	if err != nil {
		t.Fatalf("sql.Open fixture: %v", err)
	}
	tx, err := raw.BeginTx(ctx, nil)
	if err != nil {
		_ = raw.Close()
		t.Fatalf("begin malformed fixture: %v", err)
	}
	for _, stmt := range []string{
		"DROP INDEX IF EXISTS idx_graph_unresolved_generation_owner_reason",
		"ALTER TABLE graph_unresolved DROP COLUMN reason_code",
		"DROP INDEX IF EXISTS idx_doc_mentions_type",
		`CREATE TABLE doc_mentions_malformed (
			doc_path TEXT NOT NULL,
			mention_type TEXT NOT NULL,
			source_block TEXT,
			UNIQUE(doc_path, mention_type, source_block)
		)`,
		`INSERT INTO doc_mentions_malformed(doc_path, mention_type, source_block)
			SELECT doc_path, mention_type, source_block FROM doc_mentions`,
		"DROP TABLE doc_mentions",
		"ALTER TABLE doc_mentions_malformed RENAME TO doc_mentions",
		"CREATE INDEX idx_doc_mentions_type ON doc_mentions(mention_type)",
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			_ = raw.Close()
			t.Fatalf("non-capability schema change %q: %v", stmt, err)
		}
	}
	if err := tx.Commit(); err != nil {
		_ = raw.Close()
		t.Fatalf("commit malformed fixture: %v", err)
	}
	if _, err := raw.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = raw.Close()
		t.Fatalf("checkpoint malformed fixture: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close malformed fixture: %v", err)
	}

	ro, err := OpenReadOnlyExisting(fixture.root, WorkspaceDBPath(fixture.root))
	if err != nil {
		t.Fatalf("OpenReadOnlyExisting malformed fixture: %v", err)
	}
	defer ro.Close()
	snapshot, err := BeginGraphQuerySnapshot(ctx, ro, "")
	if err != nil {
		t.Fatalf("BeginGraphQuerySnapshot malformed fixture: %v", err)
	}
	_, err = snapshot.UnresolvedOmissions(ctx, 10)
	_ = snapshot.Close()
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "reason_code") {
		t.Fatalf("UnresolvedOmissions error = %v, want required-column error", err)
	}
	if _, err := ListDocMentions(ctx, ro); err == nil || !strings.Contains(strings.ToLower(err.Error()), "mention_value") {
		t.Fatalf("ListDocMentions error = %v, want required-column error", err)
	}
}
