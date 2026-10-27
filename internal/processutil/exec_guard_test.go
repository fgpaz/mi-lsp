package processutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// rawExecReferences finds constructor references, including aliases and function values.
func rawExecReferences(path string, src interface{}) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	alias := ""
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		if p == "os/exec" {
			alias = "exec"
			if imp.Name != nil {
				alias = imp.Name.Name
			}
		}
	}
	if alias == "" || alias == "_" {
		return nil, nil
	}
	var found []string
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			// Only these specific functions are allowed to construct raw commands.
			if filepath.ToSlash(path) == "internal/cli/admin.go" && fn.Name.Name == "openURL" {
				continue
			}
			if strings.HasPrefix(filepath.ToSlash(path), "internal/processutil/") && (fn.Name.Name == "Command" || fn.Name.Name == "CommandContext") {
				continue
			}
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			if alias == "." {
				if id, ok := n.(*ast.Ident); ok && (id.Name == "Command" || id.Name == "CommandContext") {
					found = append(found, fset.Position(id.Pos()).String())
				}
			} else if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == alias && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
					found = append(found, fset.Position(sel.Pos()).String())
				}
			}
			return true
		})
	}
	return found, nil
}

func TestRawExecGuardFixtures(t *testing.T) {
	cases := []struct {
		path, src string
		want      int
	}{
		{"x.go", `package x; import e "os/exec"; func f(){ _ = e.Command; _ = e.CommandContext }`, 2},
		{"x.go", `package x; import . "os/exec"; func f(){ _ = Command("git") }`, 1},
		{"internal/cli/admin.go", `package x; import "os/exec"; func openURL(){ _ = exec.Command }; func other(){ _ = exec.Command }`, 1},
		{"internal/processutil/command_windows.go", `package x; import "os/exec"; func Command(){ _ = exec.Command }; func other(){ _ = exec.Command }`, 1},
		{"x.go", `package x; import "os/exec"; func f(){ _ = exec.LookPath }`, 0},
	}
	for _, tc := range cases {
		got, err := rawExecReferences(tc.path, tc.src)
		if err != nil || len(got) != tc.want {
			t.Errorf("%s: got %v, err %v; want %d", tc.path, got, err, tc.want)
		}
	}
}

func TestProductionUsesProcessutilConstructors(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		refs, err := rawExecReferences(filepath.ToSlash(rel), data)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			t.Errorf("raw os/exec constructor: %s", ref)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
