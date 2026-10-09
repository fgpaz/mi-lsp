package grepx

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// symbolSpan is the slice of a catalog symbol the annotator needs.
type symbolSpan struct {
	name      string
	qualified string
	start     int
	end       int
}

// fileSymbols is the catalog answer for one file: whether it is indexed and
// its symbols ordered by start line.
type fileSymbols struct {
	indexed bool
	symbols []symbolSpan
}

// catalog is a read-only view of <root>/.mi-lsp/index.db. The database is
// opened lazily on the first lookup so searches that never touch indexed code
// pay nothing.
type catalog struct {
	root  string
	db    *sql.DB
	err   error
	files map[string]*fileSymbols
}

const maxCachedFiles = 4096

func newCatalog(root string) *catalog {
	return &catalog{root: root, files: make(map[string]*fileSymbols)}
}

func (c *catalog) open() error {
	if c.db != nil || c.err != nil {
		return c.err
	}
	db, err := store.OpenReadOnlyExistingWithWAL(c.root, store.WorkspaceDBPath(c.root))
	if err != nil {
		c.err = err
		return err
	}
	c.db = db
	return nil
}

func (c *catalog) close() {
	if c != nil && c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}
}

// relativePath maps a path printed by rg to the catalog key (workspace
// relative, forward slashes). ok is false for paths outside the workspace.
func (c *catalog) relativePath(cwd, printed string) (string, bool) {
	abs := printed
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, abs)
	}
	rel, err := filepath.Rel(c.root, filepath.Clean(abs))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// lookup returns the symbols of a catalog-relative file path.
func (c *catalog) lookup(ctx context.Context, rel string) (*fileSymbols, error) {
	if cached, ok := c.files[rel]; ok {
		return cached, nil
	}
	if err := c.open(); err != nil {
		return nil, err
	}
	result := &fileSymbols{}
	rows, err := c.db.QueryContext(ctx,
		`SELECT name, qualified_name, start_line, end_line FROM symbols WHERE file_path = ? ORDER BY start_line ASC, end_line DESC`,
		rel)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sym symbolSpan
		if err := rows.Scan(&sym.name, &sym.qualified, &sym.start, &sym.end); err != nil {
			_ = rows.Close()
			return nil, err
		}
		result.symbols = append(result.symbols, sym)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	if len(result.symbols) > 0 {
		result.indexed = true
	} else {
		var one int
		switch err := c.db.QueryRowContext(ctx, `SELECT 1 FROM files WHERE file_path = ?`, rel).Scan(&one); err {
		case nil:
			result.indexed = true
		case sql.ErrNoRows:
		default:
			return nil, err
		}
	}
	if len(c.files) >= maxCachedFiles {
		c.files = make(map[string]*fileSymbols)
	}
	c.files[rel] = result
	return result, nil
}

// containing returns the innermost symbol whose range covers line.
func (f *fileSymbols) containing(line int) (symbolSpan, bool) {
	best := -1
	for i, sym := range f.symbols {
		if sym.start > line {
			break
		}
		if sym.end < line {
			continue
		}
		if best < 0 || sym.end-sym.start <= f.symbols[best].end-f.symbols[best].start {
			best = i
		}
	}
	if best < 0 {
		return symbolSpan{}, false
	}
	return f.symbols[best], true
}

// startingAt returns the symbols that start exactly at line.
func (f *fileSymbols) startingAt(line int) []symbolSpan {
	var out []symbolSpan
	for _, sym := range f.symbols {
		if sym.start > line {
			break
		}
		if sym.start == line {
			out = append(out, sym)
		}
	}
	return out
}

type catalogTarget struct {
	root       string
	workspace  string
	alias      string
	registered bool
	indexed    bool
}

// resolveCatalogTarget selects the nearest local index first, then the most
// specific registered workspace even when its index is absent. It is read-only.
func resolveCatalogTarget(start string, cwd string) catalogTarget {
	dir := start
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	if absolute, err := filepath.Abs(dir); err == nil {
		dir = absolute
	}
	for current := dir; ; {
		if hasIndex(current) {
			target := targetForRoot(current, cwd)
			target.indexed = true
			return target
		}
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	if registration, ok := registeredWorkspaceForPath(dir, cwd); ok {
		return catalogTarget{root: registration.Root, workspace: registration.Name, alias: registration.Name, registered: true, indexed: hasIndex(registration.Root)}
	}
	root := inferredWorkspaceRoot(dir)
	return catalogTarget{root: root, workspace: root, indexed: hasIndex(root)}
}

func targetForRoot(root string, cwd string) catalogTarget {
	if registration, ok := registeredWorkspaceForPath(root, cwd); ok {
		return catalogTarget{root: root, workspace: registration.Name, alias: registration.Name, registered: true, indexed: hasIndex(root)}
	}
	return catalogTarget{root: root, workspace: root, indexed: hasIndex(root)}
}

func registeredWorkspaceForPath(path string, cwd string) (model.WorkspaceRegistration, bool) {
	registry, err := workspace.LoadRegistryReadOnly()
	if err != nil {
		return model.WorkspaceRegistration{}, false
	}
	bestRoot := ""
	for _, registration := range registry.Workspaces {
		root, err := filepath.Abs(filepath.Clean(registration.Root))
		if err != nil || root == "" || !pathWithin(root, path) {
			continue
		}
		if len(root) > len(bestRoot) {
			bestRoot = root
		}
	}
	if bestRoot == "" {
		return model.WorkspaceRegistration{}, false
	}
	if resolution, err := workspace.ResolveWorkspaceSelectionReadOnly(bestRoot, cwd); err == nil {
		return resolution.Registration, true
	}
	aliases := make([]string, 0)
	for alias, registration := range registry.Workspaces {
		root, err := filepath.Abs(filepath.Clean(registration.Root))
		if err == nil && root == bestRoot {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	if len(aliases) == 0 {
		return model.WorkspaceRegistration{}, false
	}
	registration := registry.Workspaces[aliases[0]]
	registration.Name = aliases[0]
	return registration, true
}

func inferredWorkspaceRoot(dir string) string {
	for current := dir; ; {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	if detected, err := workspace.DetectWorkspace(dir); err == nil {
		return detected.Root
	}
	return dir
}

func catalogTargetNotice(target catalogTarget) string {
	if target.registered && target.alias != "" {
		if !target.indexed {
			return fmt.Sprintf("mi-lsp: índice ausente para workspace %q; corré `mi-lsp index --workspace %s`", target.alias, shellQuote(target.alias))
		}
		return fmt.Sprintf("mi-lsp: índice no disponible para workspace %q; verificá `mi-lsp workspace status %s --full` o corré `mi-lsp index --workspace %s`", target.alias, shellQuote(target.alias), shellQuote(target.alias))
	}
	if target.root == "" {
		return NoIndexNotice
	}
	alias := filepath.Base(filepath.Clean(target.root))
	if alias == "" || alias == "." || alias == string(filepath.Separator) {
		alias = "workspace"
	}
	return fmt.Sprintf("mi-lsp: workspace no registrado; corré `mi-lsp workspace add %s --name %s --no-index` y luego `mi-lsp index --workspace %s`", shellQuote(target.root), shellQuote(alias), shellQuote(alias))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func hasIndex(root string) bool {
	info, err := os.Stat(store.WorkspaceDBPath(root))
	return err == nil && !info.IsDir()
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
