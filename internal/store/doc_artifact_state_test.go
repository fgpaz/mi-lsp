package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestUpsertDocArtifactState_RoundTrip(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	state := model.DocArtifactState{
		Path:                "wiki/00_test.md",
		Size:                1024,
		MtimeNsec:           1234567890,
		ContentSHA256:       "abc123",
		ParserVersion:       model.ParserVersion,
		AuthorityConfigHash: "config-hash",
		IndexedAt:           1234567890,
		LastDocsGenAt:       1234567890,
		Lifecycle:           model.DocLifecycleActive,
	}
	if err := UpsertDocArtifactState(ctx, db, state); err != nil {
		t.Fatalf("UpsertDocArtifactState: %v", err)
	}

	got, ok, err := GetDocArtifactState(ctx, db, "wiki/00_test.md")
	if err != nil {
		t.Fatalf("GetDocArtifactState: %v", err)
	}
	if !ok {
		t.Fatalf("expected state to exist")
	}
	if got.Path != state.Path || got.ContentSHA256 != state.ContentSHA256 || got.Lifecycle != state.Lifecycle {
		t.Fatalf("state mismatch: got %#v, want %#v", got, state)
	}
}

func TestUpsertDocArtifactState_UpdatesExisting(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	state := model.DocArtifactState{
		Path:            "wiki/00_test.md",
		ContentSHA256:   "hash-1",
		LastDocsGenAt:   1,
		Lifecycle:       model.DocLifecycleActive,
	}
	if err := UpsertDocArtifactState(ctx, db, state); err != nil {
		t.Fatalf("Upsert first: %v", err)
	}

	state.ContentSHA256 = "hash-2"
	state.LastDocsGenAt = 2
	if err := UpsertDocArtifactState(ctx, db, state); err != nil {
		t.Fatalf("Upsert second: %v", err)
	}

	got, ok, err := GetDocArtifactState(ctx, db, "wiki/00_test.md")
	if err != nil || !ok {
		t.Fatalf("GetDocArtifactState: %v, %v", err, ok)
	}
	if got.ContentSHA256 != "hash-2" {
		t.Fatalf("content hash should be updated, got %q", got.ContentSHA256)
	}
	if got.LastDocsGenAt != 2 {
		t.Fatalf("last docs gen should be updated, got %d", got.LastDocsGenAt)
	}
}

func TestDeleteDocArtifactState_RemovesState(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	state := model.DocArtifactState{
		Path:          "wiki/00_test.md",
		ContentSHA256: "hash",
		Lifecycle:     model.DocLifecycleActive,
	}
	if err := UpsertDocArtifactState(ctx, db, state); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := DeleteDocArtifactState(ctx, db, "wiki/00_test.md"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, ok, err := GetDocArtifactState(ctx, db, "wiki/00_test.md")
	if err != nil {
		t.Fatalf("GetDocArtifactState: %v", err)
	}
	if ok {
		t.Fatalf("expected state to be deleted")
	}
}

func TestListDocArtifactStates_ReturnsAll(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	states := []model.DocArtifactState{
		{Path: "wiki/a.md", ContentSHA256: "a", Lifecycle: model.DocLifecycleActive},
		{Path: "wiki/b.md", ContentSHA256: "b", Lifecycle: model.DocLifecycleActive},
	}
	for _, s := range states {
		if err := UpsertDocArtifactState(ctx, db, s); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	all, err := ListDocArtifactStates(ctx, db)
	if err != nil {
		t.Fatalf("ListDocArtifactStates: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 states, got %d", len(all))
	}
}

func TestNormalizeRepoRelative(t *testing.T) {
	root := "/workspace"
	absPath := "/workspace/docs/wiki/test.md"
	rel := NormalizeRepoRelative(root, absPath)
	if rel != "docs/wiki/test.md" {
		t.Fatalf("NormalizeRepoRelative(%q, %q) = %q, want %q", root, absPath, rel, "docs/wiki/test.md")
	}
}

func TestIsDiskAbsent_ExistingFile(t *testing.T) {
	root := t.TempDir()
	testFile := root + "/test.md"
	if err := writeFile(testFile, "# test"); err != nil {
		t.Fatal(err)
	}
	if IsDiskAbsent(testFile) {
		t.Fatalf("IsDiskAbsent(%q) should be false for existing file", testFile)
	}
}

func TestIsDiskAbsent_NonExistingFile(t *testing.T) {
	root := t.TempDir()
	testFile := root + "/nonexistent.md"
	if !IsDiskAbsent(testFile) {
		t.Fatalf("IsDiskAbsent(%q) should be true for non-existing file", testFile)
	}
}

func TestAuthorityConfigDigest_Deterministic(t *testing.T) {
	d1 := AuthorityConfigDigest(model.ProjectFile{}, nil)
	d2 := AuthorityConfigDigest(model.ProjectFile{}, nil)
	if d1 != d2 {
		t.Fatalf("AuthorityConfigDigest should be deterministic: got %s, %s", d1, d2)
	}
}

func TestAuthorityConfigDigestForProfile_InvalidatesOnConfigChange(t *testing.T) {
	base := model.DocsReadProfile{Version: 1, GenericDocs: model.DocsGenericFallback{Paths: []string{"docs/"}}}
	changed := base
	changed.GenericDocs.Paths = []string{"wiki/"}
	if AuthorityConfigDigestForProfile(base, nil) == AuthorityConfigDigestForProfile(changed, nil) {
		t.Fatal("profile path changes must invalidate the authority digest")
	}
}

// === RacilyClean Tests (Repair 5) ===

func TestRacilyClean_UnchangedOldSkipsBodyRead(t *testing.T) {
	root := t.TempDir()
	testFile := filepath.Join(root, "old.md")
	content := []byte("stable")
	if err := os.WriteFile(testFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(testFile, old, old); err != nil {
		t.Fatal(err)
	}
	st, err := statFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(content)
	reads := 0
	got, reused, err := RacilyClean(context.Background(), testFile, func() ([]byte, error) {
		reads++
		return os.ReadFile(testFile)
	}, model.DocArtifactState{Path: "old.md", Size: st.size, MtimeNsec: st.mtimeNsec, ContentSHA256: hex.EncodeToString(hash[:]), IndexedAt: time.Now().Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if !reused || got != "" || reads != 0 {
		t.Fatalf("old unchanged result=(%q,%v), reads=%d; want reuse without read", got, reused, reads)
	}
}

func TestRacilyClean_UnchangedReuse(t *testing.T) {
	root := t.TempDir()
	testFile := filepath.Join(root, "unchanged.md")
	if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(testFile, old, old); err != nil {
		t.Fatal(err)
	}
	st, err := statFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	stored := model.DocArtifactState{
		Path:      "unchanged.md",
		Size:      st.size,
		MtimeNsec: st.mtimeNsec,
		IndexedAt: time.Now().Unix(),
	}
	h := sha256.Sum256([]byte("content"))
	stored.ContentSHA256 = hex.EncodeToString(h[:])

	ctx := context.Background()
	hash, reused, err := RacilyClean(ctx, testFile, func() ([]byte, error) {
		return os.ReadFile(testFile)
	}, stored)
	if err != nil {
		t.Fatalf("RacilyClean: %v", err)
	}
	if !reused {
		t.Fatal("expected reuse for unchanged file")
	}
	if hash != "" {
		t.Fatalf("expected empty hash on reuse, got %q", hash)
	}
}

func TestRacilyClean_MissingIndexedAtDoesNotTrustMetadata(t *testing.T) {
	root := t.TempDir()
	testFile := filepath.Join(root, "missing-index-time.md")
	content := []byte("stable")
	if err := os.WriteFile(testFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(testFile, old, old); err != nil {
		t.Fatal(err)
	}
	st, err := statFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(content)
	reads := 0
	_, reused, err := RacilyClean(context.Background(), testFile, func() ([]byte, error) {
		reads++
		return os.ReadFile(testFile)
	}, model.DocArtifactState{Path: "missing-index-time.md", Size: st.size, MtimeNsec: st.mtimeNsec, ContentSHA256: hex.EncodeToString(hash[:])})
	if err != nil {
		t.Fatal(err)
	}
	if !reused || reads != 1 {
		t.Fatalf("missing IndexedAt result reused=%v reads=%d; want hashed verification", reused, reads)
	}
}

func TestRacilyClean_SameSizeSameMtimeRecentRewrite(t *testing.T) {
	root := t.TempDir()
	testFile := filepath.Join(root, "rewrite.md")
	content := []byte("original")
	if err := os.WriteFile(testFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := statFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	stored := model.DocArtifactState{
		Path:         "rewrite.md",
		Size:         st.size,
		MtimeNsec:    st.mtimeNsec,
		ContentSHA256: "oldhash",
		// A same-second index timestamp is too coarse to trust metadata alone.
		IndexedAt: time.Unix(0, st.mtimeNsec).Unix(),
	}

	ctx := context.Background()
	readCalled := false
	hash, reused, err := RacilyClean(ctx, testFile, func() ([]byte, error) {
		readCalled = true
		// Simulate a recent same-size rewrite while metadata remains equal.
		return []byte("rewriten"), nil
	}, stored)
	if err != nil {
		t.Fatalf("RacilyClean: %v", err)
	}
	if reused {
		t.Fatal("expected no reuse when content differs")
	}
	if hash == "" {
		t.Fatal("expected hash for rewritten content")
	}
	if !readCalled {
		t.Fatal("read function should have been called")
	}
}

func TestRacilyClean_StableChangedFile(t *testing.T) {
	root := t.TempDir()
	testFile := filepath.Join(root, "changed.md")
	if err := os.WriteFile(testFile, []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := statFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	// Store old mtime/size (simulating a known changed file)
	stored := model.DocArtifactState{
		Path:        "changed.md",
		Size:        st.size - 100, // different size
		MtimeNsec:   st.mtimeNsec - 1000, // different mtime
		ContentSHA256: "oldhash",
	}

	ctx := context.Background()
	hash, reused, err := RacilyClean(ctx, testFile, func() ([]byte, error) {
		return os.ReadFile(testFile)
	}, stored)
	if err != nil {
		t.Fatalf("RacilyClean: %v", err)
	}
	if reused {
		t.Fatal("expected no reuse for changed file")
	}
	if hash == "" {
		t.Fatal("expected hash for changed file")
	}
}

func TestRacilyClean_SecondConcurrentMutation(t *testing.T) {
	root := t.TempDir()
	testFile := filepath.Join(root, "racy.md")
	if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := statFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	stored := model.DocArtifactState{Path: "racy.md", Size: st.size, MtimeNsec: st.mtimeNsec, ContentSHA256: "storedhash"}

	previousStat := racilyCleanStatPath
	calls := 0
	stamps := []mtimeAndSize{
		{mtimeNsec: st.mtimeNsec, size: st.size},
		{mtimeNsec: st.mtimeNsec + 1, size: st.size + 1},
		{mtimeNsec: st.mtimeNsec + 1, size: st.size + 1},
		{mtimeNsec: st.mtimeNsec + 2, size: st.size + 2},
	}
	racilyCleanStatPath = func(string) (mtimeAndSize, error) {
		calls++
		return stamps[calls-1], nil
	}
	t.Cleanup(func() { racilyCleanStatPath = previousStat })

	reads := 0
	_, _, err = RacilyClean(context.Background(), testFile, func() ([]byte, error) {
		reads++
		return []byte("content"), nil
	}, stored)
	if err == nil {
		t.Fatal("expected ErrConcurrentChange for second concurrent mutation")
	}
	if _, ok := err.(*model.ErrConcurrentChange); !ok {
		t.Fatalf("expected *ErrConcurrentChange, got %T: %v", err, err)
	}
	if reads != 2 {
		t.Fatalf("expected exactly one retry (2 reads), got %d", reads)
	}
	if calls != len(stamps) {
		t.Fatalf("stat calls=%d, want %d deterministic before/after stamps", calls, len(stamps))
	}
}

func TestRacilyClean_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately.
	cancel()

	stored := model.DocArtifactState{
		Path:      "test.md",
		Size:      0,
		MtimeNsec: 0,
	}

	_, _, err := RacilyClean(ctx, "/nonexistent.md", func() ([]byte, error) {
		return nil, os.ErrNotExist
	}, stored)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

// writeFile is a test helper.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
