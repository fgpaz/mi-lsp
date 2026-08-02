package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTP14PreparationReceiptPortableMetadata(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "receipt.json")
	r, err := WritePreparationReceipt(p, d, d, []string{"src"}, "/packet.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Schema != PreparationReceiptSchema || r.PreparationID == "" || r.PacketPath != p || r.Digest == "" || !r.Transferable {
		t.Fatalf("bad receipt: %+v", r)
	}
	b, _ := os.ReadFile(p)
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["content"]; ok {
		t.Fatal("receipt contains content")
	}
}

func TestTP15SeedFailureIsReparableEvidenceOnly(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "receipt.json")
	r, err := WritePreparationReceipt(p, d, d, nil, "", os.ErrNotExist)
	if err != nil {
		t.Fatal(err)
	}
	if !r.EvidenceOnly || r.Repairability != "refresh_required" || r.RecommendedAction != "refresh" || r.Transferable {
		t.Fatalf("bad failure receipt: %+v", r)
	}
}

func TestTP16IsolatedSkillsRootsAndCatalogs(t *testing.T) {
	d := t.TempDir()
	rootA, rootB := filepath.Join(d, "skills-a"), filepath.Join(d, "skills-b")
	for _, tc := range []struct{ root, id, phrase string }{{rootA, "skill-a", "unique catalog alpha phrase"}, {rootB, "skill-b", "unique catalog beta phrase"}} {
		dir := filepath.Join(tc.root, tc.id)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + tc.id + "\ndescription: " + tc.phrase + "\n---\n# " + tc.id + "\n" + tc.phrase + "\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	global := filepath.Join(d, "global.json")
	if err := os.WriteFile(global, []byte("global sentinel"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCatalogPath, global)
	catAPath, catBPath := filepath.Join(d, "catalog-a.json"), filepath.Join(d, "catalog-b.json")
	catA, _, err := IndexAndSave(context.Background(), IndexOptions{SkillsRoot: rootA, CatalogPath: catAPath})
	if err != nil {
		t.Fatal(err)
	}
	catB, _, err := IndexAndSave(context.Background(), IndexOptions{SkillsRoot: rootB, CatalogPath: catBPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(catA.Skills) != 1 || catA.Skills[0].ID != "skill-a" || len(catB.Skills) != 1 || catB.Skills[0].ID != "skill-b" {
		t.Fatalf("catalog isolation failed: %#v %#v", catA.Skills, catB.Skills)
	}
	gb, _ := os.ReadFile(global)
	if string(gb) != "global sentinel" {
		t.Fatal("global catalog changed")
	}
}

func TestTP17IndexAndSearchUseSameSelectedCatalog(t *testing.T) {
	d := t.TempDir()
	root := filepath.Join(d, "skills")
	skillDir := filepath.Join(root, "selected")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: selected\ndescription: exact unique catalog phrase\n---\nexact unique catalog phrase\n"), 0644); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(d, "selected.json")
	cat, _, err := IndexAndSave(context.Background(), IndexOptions{SkillsRoot: root, CatalogPath: catalogPath})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(catalogPath)
	sum := sha256.Sum256(before)
	loaded, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	hits, _ := Search(context.Background(), loaded, SearchOptions{Query: "exact unique catalog phrase", TopK: 5})
	if len(hits) == 0 || hits[0].Skill.ID != "selected" || !strings.HasSuffix(hits[0].Skill.SourcePath, filepath.Join("selected", "SKILL.md")) {
		t.Fatalf("unexpected hits: %#v", hits)
	}
	after, _ := os.ReadFile(catalogPath)
	got := sha256.Sum256(after)
	if hex.EncodeToString(sum[:]) != hex.EncodeToString(got[:]) {
		t.Fatal("selected catalog changed during load/search")
	}
	if len(cat.Skills) != 1 {
		t.Fatal(cat)
	}
}
