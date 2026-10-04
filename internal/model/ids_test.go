package model

import (
	"strings"
	"testing"
)

func TestRevOfNormalizaSaltosDeLinea(t *testing.T) {
	a := RevOf("a\r\nb\r\nc")
	b := RevOf("a\nb\nc")
	c := RevOf("a\rb\rc")
	if a != b || b != c {
		t.Fatalf("rev debe ignorar CRLF/CR: %s %s %s", a, b, c)
	}
	if len(a) != IDRevLen {
		t.Fatalf("rev debe tener %d hex, got %q", IDRevLen, a)
	}
	if RevOf("a\nb") == RevOf("a\nB") {
		t.Fatal("contenido distinto debe dar rev distinto")
	}
	if RevOfLines([]string{"a", "b", "c"}) != b {
		t.Fatal("RevOfLines debe coincidir con RevOf del texto unido")
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	cases := []string{
		"s1:internal/service/app.go#App.Execute@3f9a01c2",
		"s1:internal/service/app.go#App.Execute~ab12@3f9a01c2",
		"s1:internal/service/app.go#App.Execute",
		"r1:internal/cli/nav.go:313-340@77b0e4aa",
		"r1:internal/cli/nav.go:313-340",
		"d1:RF-QRY-001@a0c3d991",
		"d1:RF-QRY-001",
	}
	for _, raw := range cases {
		id, err := ParseID(raw)
		if err != nil {
			t.Fatalf("ParseID(%q): %v", raw, err)
		}
		if got := id.String(); got != raw {
			t.Fatalf("round trip %q -> %q", raw, got)
		}
	}
}

func TestParseIDCampos(t *testing.T) {
	id, err := ParseID("s1:internal/a.go#T.M~0f1e@deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if id.Scheme != "s1" || id.File != "internal/a.go" || id.Name != "T.M" || id.Sig != "0f1e" || id.Rev != "deadbeef" {
		t.Fatalf("campos inesperados: %+v", id)
	}
	r, err := ParseID("r1:a/b.go:3-9@00112233")
	if err != nil || r.File != "a/b.go" || r.Start != 3 || r.End != 9 {
		t.Fatalf("r1 inesperado: %+v err=%v", r, err)
	}
	if id.Base() != "s1:internal/a.go#T.M~0f1e" {
		t.Fatalf("Base = %q", id.Base())
	}
}

func TestParseIDErrores(t *testing.T) {
	for _, raw := range []string{"", "x1:a", "s1:", "s1:nofile", "s1:#name", "s1:file#", "r1:a.go", "r1:a.go:9-3", "r1:a.go:0-3", "r1:a.go:x-y", "zz"} {
		if _, err := ParseID(raw); err == nil {
			t.Fatalf("ParseID(%q) debía fallar", raw)
		}
		if IsID(raw) {
			t.Fatalf("IsID(%q) debía ser false", raw)
		}
	}
}

func TestSymbolQNameYID(t *testing.T) {
	rec := SymbolRecord{FilePath: "internal/service/app.go", Name: "Execute", Parent: "App", QualifiedName: "internal/service/app.go::App.Execute", SignatureHash: "ABCDEF0123"}
	if got := SymbolQName(rec); got != "App.Execute" {
		t.Fatalf("qname = %q", got)
	}
	if got := SymbolID(rec, "3f9a01c2", false).String(); got != "s1:internal/service/app.go#App.Execute@3f9a01c2" {
		t.Fatalf("id = %q", got)
	}
	if got := SymbolID(rec, "3f9a01c2", true).String(); !strings.Contains(got, "~abcd@") {
		t.Fatalf("id con sig = %q", got)
	}
	bare := SymbolRecord{FilePath: "a.go", Name: "F"}
	if SymbolQName(bare) != "F" {
		t.Fatal("sin parent ni qualified_name debe usar name")
	}
	if SymbolQName(SymbolRecord{Name: "M", Parent: "T"}) != "T.M" {
		t.Fatal("sin qualified_name debe armar parent.name")
	}
}

func TestResolveSymbolID(t *testing.T) {
	cands := []SymbolRecord{
		{FilePath: "a.go", Name: "F", QualifiedName: "a.go::F", StartLine: 1, SignatureHash: "aaaa1111"},
		{FilePath: "a.go", Name: "G", QualifiedName: "a.go::G", StartLine: 5, SignatureHash: "bbbb2222"},
		{FilePath: "b.go", Name: "F", QualifiedName: "b.go::F", StartLine: 1},
	}
	revs := map[int]string{1: "11111111", 5: "22222222"}
	revOf := func(r SymbolRecord) string { return revs[r.StartLine] }

	rec, rev, state := ResolveSymbolID(ID{Scheme: "s1", File: "a.go", Name: "F", Rev: "11111111"}, cands, revOf)
	if state != IDStateFresh || rec.Name != "F" || rec.FilePath != "a.go" || rev != "11111111" {
		t.Fatalf("fresh: %v %v %v", rec, rev, state)
	}
	_, rev, state = ResolveSymbolID(ID{Scheme: "s1", File: "a.go", Name: "F", Rev: "99999999"}, cands, revOf)
	if state != IDStateStale || rev != "11111111" {
		t.Fatalf("stale: %v %v", rev, state)
	}
	_, rev, state = ResolveSymbolID(ID{Scheme: "s1", File: "a.go", Name: "F"}, cands, revOf)
	if state != IDStateFresh || rev != "11111111" {
		t.Fatalf("sin rev debe resolver fresh con su rev: %v %v", rev, state)
	}
	_, _, state = ResolveSymbolID(ID{Scheme: "s1", File: "a.go", Name: "Nope", Rev: "11111111"}, cands, revOf)
	if state != IDStateMissing {
		t.Fatalf("missing: %v", state)
	}
}

func TestResolveSymbolIDSobrecargas(t *testing.T) {
	cands := []SymbolRecord{
		{FilePath: "a.cs", Name: "M", Parent: "C", QualifiedName: "a.cs::C.M", StartLine: 1, SignatureHash: "aaaa0000"},
		{FilePath: "a.cs", Name: "M", Parent: "C", QualifiedName: "a.cs::C.M", StartLine: 9, SignatureHash: "bbbb0000"},
	}
	revOf := func(r SymbolRecord) string {
		if r.StartLine == 1 {
			return "11111111"
		}
		return "22222222"
	}
	rec, _, state := ResolveSymbolID(ID{Scheme: "s1", File: "a.cs", Name: "C.M", Sig: "bbbb", Rev: "22222222"}, cands, revOf)
	if state != IDStateFresh || rec.StartLine != 9 {
		t.Fatalf("sig4 debe elegir la sobrecarga: %+v %v", rec, state)
	}
	rec, _, state = ResolveSymbolID(ID{Scheme: "s1", File: "a.cs", Name: "C.M", Rev: "11111111"}, cands, revOf)
	if state != IDStateFresh || rec.StartLine != 1 {
		t.Fatalf("sin sig debe elegir por rev: %+v %v", rec, state)
	}
}
