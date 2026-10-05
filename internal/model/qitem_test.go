package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQItemProjectionDefaultOrden(t *testing.T) {
	it := QItem{ID: "s1:a.go#F@11111111", Kind: "func", Name: "F", File: "a.go", Line: 4, Origin: "catalog", Lang: "go"}
	raw, err := json.Marshal(it.Project(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"s1:a.go#F@11111111","kind":"func","name":"F","file":"a.go","line":4,"origin":"catalog"}`
	if string(raw) != want {
		t.Fatalf("got %s", raw)
	}
}

func TestQItemProjectionFieldsYMarcadores(t *testing.T) {
	it := QItem{ID: "s1:a.go#F@11111111", Name: "F", File: "a.go", Line: 4, Stale: true, Seen: true, Text: "x y"}
	row := it.Project([]string{"name", "text"})
	raw, _ := json.Marshal(row)
	if string(raw) != `{"name":"F","text":"x y","rev":"rev:11111111","stale":true,"seen":true}` {
		t.Fatalf("got %s", raw)
	}
	if _, ok := row.Get("file"); ok {
		t.Fatal("file no debía proyectarse")
	}
}

func TestQItemMissing(t *testing.T) {
	raw, _ := json.Marshal(QItem{ID: "s1:a.go#Gone@11111111", Missing: true}.Project(nil))
	if string(raw) != `{"id":"s1:a.go#Gone@11111111","missing":true}` {
		t.Fatalf("got %s", raw)
	}
}

func TestQRowLine(t *testing.T) {
	it := QItem{ID: "s1:a.go#F@11111111", Kind: "func", Name: "F", File: "a.go", Line: 4, Origin: "catalog", Edge: &QEdge{Dir: "callers", From: "s1:b.go#G@22222222", Depth: 1}}
	line := it.Project(nil).Line()
	if !strings.HasPrefix(line, "s1:a.go#F@11111111  func  F  a.go:4") || !strings.Contains(line, "edge=callers<") {
		t.Fatalf("line = %q", line)
	}
	text := QItem{ID: "r1:a.go:1-2@11111111", File: "a.go", Line: 1, Text: "l1\nl2", Read: true}.Project(nil).Line()
	if !strings.Contains(text, "\n    l1\n    l2") {
		t.Fatalf("texto con sangría: %q", text)
	}
}

func TestQFieldValidYMoreSevere(t *testing.T) {
	if !QFieldValid("rev") || QFieldValid("zzz") {
		t.Fatal("QFieldValid")
	}
	if MoreSevereReason("", ReasonNoMatches) != ReasonNoMatches {
		t.Fatal("vacío pierde")
	}
	if MoreSevereReason(ReasonNoMatches, ReasonGraphUnavailable) != ReasonGraphUnavailable {
		t.Fatal("graph_unavailable > no_matches")
	}
	if MoreSevereReason(ReasonIndexNotReady, ReasonNoMatches) != ReasonIndexNotReady {
		t.Fatal("index_not_ready > no_matches")
	}
}

func TestSortQItems(t *testing.T) {
	items := []QItem{{Name: "b", Line: 3}, {Name: "a", Line: 9}, {Name: "c", Line: 1}}
	SortQItems(items, "name", false)
	if items[0].Name != "a" || items[2].Name != "c" {
		t.Fatalf("name asc: %+v", items)
	}
	SortQItems(items, "line", true)
	if items[0].Line != 9 {
		t.Fatalf("line desc: %+v", items)
	}
}
