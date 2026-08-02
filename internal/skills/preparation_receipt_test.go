package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTP15PreparationReceiptPortableMetadata(t *testing.T) {
	p := filepath.Join(t.TempDir(), "receipt.json")
	r, err := WritePreparationReceipt(p, "/workspace", "/evidence", []string{"src"}, "/packet.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Schema != PreparationReceiptSchema || r.PreparationID == "" || r.PacketPath != "/packet.json" || r.Digest == "" || !r.Transferable {
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

func TestTP16SeedFailureIsReparableEvidenceOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "receipt.json")
	r, err := WritePreparationReceipt(p, "w", "e", nil, "", os.ErrNotExist)
	if err != nil {
		t.Fatal(err)
	}
	if !r.EvidenceOnly || r.Repairability != "reparable" || r.RecommendedAction == "" || !r.Transferable {
		t.Fatalf("bad failure receipt: %+v", r)
	}
}

func TestTP17ReceiptWritesOnlySelectedPath(t *testing.T) {
	d := t.TempDir()
	selected := filepath.Join(d, "selected", "catalog.json")
	global := filepath.Join(d, "global.json")
	if err := os.WriteFile(global, []byte("sentinel"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := IndexAndSave(t.Context(), IndexOptions{SkillsRoot: d, CatalogPath: selected}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(global)
	if string(b) != "sentinel" {
		t.Fatal("global catalog changed")
	}
	if _, err := os.Stat(selected); err != nil {
		t.Fatal(err)
	}
}
