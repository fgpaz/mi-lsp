package service

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/fgpaz/mi-lsp/internal/model"
	"golang.org/x/text/unicode/norm"
)

const kineticCaseLineCap = 200

var (
	kineticActionWords = []string{"proponer", "juzgar", "promover", "reemplazar", "descartar"}
	kineticTypeWords   = []string{"nota", "propuesta", "decision", "fuente"}
	kineticDecisionRef = regexp.MustCompile(`D-[0-9]+`)
	kineticWord        = regexp.MustCompile(`\b` + `[a-z0-9]+` + `\b`)
)

type kineticPair struct {
	Modelo   string
	Acciones string
}

type kineticRow struct {
	Line  int
	Cells map[string]string
}

type kineticCase struct {
	TS     string `json:"ts"`
	Quien  string `json:"quien"`
	Rol    string `json:"rol"`
	Accion string `json:"accion"`
	Que    string `json:"que"`
	File   string `json:"-"`
	Line   int    `json:"-"`
}

func (a *App) kineticIntent(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, question string, pair kineticPair) model.Envelope {
	folded := foldKinetic(question)
	actions := readKineticTable(registration.Root, pair.Acciones)
	modelRows := readKineticTable(registration.Root, pair.Modelo)
	cases := readKineticCases(kineticCasesDir(request))

	var items []map[string]any
	var warnings []string
	switch {
	case kineticHasWord(folded, kineticActionWords):
		name := kineticFirstWord(folded, kineticActionWords)
		if row, ok := kineticRowBy(actions, "accion", name); ok {
			items = append(items, kineticActionItem(pair.Acciones, row))
		}
		for _, c := range cases {
			if foldKinetic(c.Accion) != name {
				continue
			}
			items = append(items, kineticCaseItem(c))
			items = append(items, kineticThingsFromQue(registration, c.Que)...)
		}
	case kineticDecisionRef.MatchString(question):
		id := kineticDecisionRef.FindString(question)
		if cosa, ok := kineticDecisionThing(registration, id); ok {
			items = append(items, cosa)
		}
		for _, c := range cases {
			if !strings.Contains(c.Que, id) {
				continue
			}
			items = append(items, kineticCaseItem(c))
			if row, ok := kineticRowBy(actions, "accion", foldKinetic(c.Accion)); ok {
				items = append(items, kineticActionItem(pair.Acciones, row))
			}
		}
	case kineticHasWord(folded, kineticTypeWords):
		name := kineticFirstWord(folded, kineticTypeWords)
		if row, ok := kineticRowBy(modelRows, "tipo", name); ok {
			items = append(items, kineticCosaRow(pair.Modelo, row))
		}
		for _, row := range actions {
			items = append(items, kineticActionItem(pair.Acciones, row))
		}
	default:
		docs, warn := a.kineticDocsItems(ctx, request, registration, question)
		items = docs
		if warn != "" {
			warnings = append(warnings, warn)
		}
	}

	status := "al_dia"
	pages := []map[string]any{}
	if kineticCatalogDrift(cases, actions) {
		status = "drift"
		pages = []map[string]any{{"path": pair.Acciones, "reason": "accion_fuera_de_catalogo"}}
	}
	items = append(items, map[string]any{
		"result_kind": "wiki_close",
		"status":      status,
		"watch":       []string{pair.Modelo, pair.Acciones},
		"pages":       pages,
	})
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "intent",
		Mode:      "docs",
		Items:     items,
		Warnings:  warnings,
	}
}

func (a *App) kineticDocsItems(ctx context.Context, request model.CommandRequest, registration model.WorkspaceRegistration, question string) ([]map[string]any, string) {
	scopedRepo, scopeWarnings, scopeEnvelope := resolveCatalogRepoScope(registration, model.ProjectFile{}, request.Payload)
	if scopeEnvelope != nil {
		return nil, "kinetic docs path unavailable"
	}
	env, err := a.intentDocs(ctx, request, registration, question, 10, 0, scopedRepo, scopeWarnings)
	if err != nil || env.Mode == "code" {
		return nil, "kinetic docs path unavailable"
	}
	items, _ := env.Items.([]map[string]any)
	return items, ""
}

func kineticProject(root string) (kineticPair, bool) {
	if kineticProductRoot(root) {
		return kineticPair{}, false
	}
	wiki := filepath.Join(root, "wiki")
	var dirs []string
	_ = filepath.WalkDir(wiki, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if fileExists(filepath.Join(path, "modelo.md")) && fileExists(filepath.Join(path, "acciones.md")) {
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil {
				dirs = append(dirs, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if len(dirs) == 0 {
		return kineticPair{}, false
	}
	sort.Slice(dirs, func(i, j int) bool {
		iHit := strings.Contains(dirs[i], "10-conceptos")
		jHit := strings.Contains(dirs[j], "10-conceptos")
		if iHit != jHit {
			return iHit
		}
		if len(dirs[i]) != len(dirs[j]) {
			return len(dirs[i]) < len(dirs[j])
		}
		return dirs[i] < dirs[j]
	})
	dir := dirs[0]
	return kineticPair{
		Modelo:   dir + "/modelo.md",
		Acciones: dir + "/acciones.md",
	}, true
}

func kineticProductRoot(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	if kineticHasManifest(entries) {
		return true
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		kids, err := os.ReadDir(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		if kineticHasManifest(kids) {
			return true
		}
	}
	return false
}

func kineticHasManifest(entries []os.DirEntry) bool {
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch name {
		case "go.mod", "package.json", "Cargo.toml":
			return true
		}
		if strings.HasSuffix(name, ".csproj") {
			return true
		}
	}
	return false
}

func foldKinetic(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = norm.NFD.String(value)
	var b strings.Builder
	for _, r := range value {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func kineticHasWord(folded string, words []string) bool {
	return kineticFirstWord(folded, words) != ""
}

func kineticFirstWord(folded string, words []string) string {
	have := map[string]bool{}
	for _, match := range kineticWord.FindAllString(folded, -1) {
		have[match] = true
	}
	for _, word := range words {
		if have[word] {
			return word
		}
	}
	return ""
}

func readKineticTable(root, rel string) []kineticRow {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil || len(data) > 2<<20 {
		return nil
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	var header []string
	var rows []kineticRow
	for i, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "|") {
			continue
		}
		cells := splitMarkdownCells(trim)
		if len(cells) == 0 {
			continue
		}
		if kineticSeparator(cells) {
			continue
		}
		if header == nil {
			header = make([]string, len(cells))
			for j, cell := range cells {
				header[j] = foldKinetic(cell)
			}
			continue
		}
		row := kineticRow{Line: i + 1, Cells: map[string]string{}}
		for j, cell := range cells {
			if j < len(header) {
				row.Cells[header[j]] = strings.TrimSpace(cell)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func splitMarkdownCells(line string) []string {
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return nil
	}
	parts = parts[1 : len(parts)-1]
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = strings.TrimSpace(part)
	}
	return out
}

func kineticSeparator(cells []string) bool {
	for _, cell := range cells {
		trim := strings.TrimSpace(cell)
		if trim == "" {
			return true
		}
		for _, r := range trim {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func kineticRowBy(rows []kineticRow, column, foldedValue string) (kineticRow, bool) {
	for _, row := range rows {
		if foldKinetic(row.Cells[column]) == foldedValue {
			return row, true
		}
	}
	return kineticRow{}, false
}

func kineticActionItem(file string, row kineticRow) map[string]any {
	return map[string]any{
		"result_kind":  "accion",
		"nombre":       row.Cells["accion"],
		"parametros":   row.Cells["parametros"],
		"precondicion": row.Cells["precondicion"],
		"efecto":       row.Cells["efecto"],
		"rol":          row.Cells["rol"],
		"file":         file,
		"line":         row.Line,
	}
}

func kineticCosaRow(file string, row kineticRow) map[string]any {
	return map[string]any{
		"result_kind": "cosa",
		"nombre":      row.Cells["tipo"],
		"que_es":      row.Cells["que es"],
		"campos":      row.Cells["campos"],
		"file":        file,
		"line":        row.Line,
	}
}

func kineticCaseItem(c kineticCase) map[string]any {
	return map[string]any{
		"result_kind": "caso",
		"ts":          c.TS,
		"quien":       c.Quien,
		"rol":         c.Rol,
		"accion":      c.Accion,
		"que":         c.Que,
		"file":        c.File,
		"line":        c.Line,
	}
}

func kineticThingsFromQue(registration model.WorkspaceRegistration, que string) []map[string]any {
	var items []map[string]any
	seen := map[string]bool{}
	for _, id := range kineticDecisionRef.FindAllString(que, -1) {
		if seen[id] {
			continue
		}
		seen[id] = true
		if cosa, ok := kineticDecisionThing(registration, id); ok {
			items = append(items, cosa)
			continue
		}
		items = append(items, map[string]any{
			"result_kind": "cosa",
			"decision_id": id,
			"origin":      "caso",
		})
	}
	return items
}

func kineticDecisionThing(registration model.WorkspaceRegistration, id string) (map[string]any, bool) {
	mapPath := filepath.Join(registration.Root, filepath.FromSlash(decisionMapRel))
	data, err := os.ReadFile(mapPath)
	if err != nil || len(data) == 0 || len(data) > 2<<20 {
		return nil, false
	}
	rel, start, end, ok := lookupDecisionRange(string(data), id)
	if !ok {
		return nil, false
	}
	abs, inside := workspaceFile(registration.Root, rel)
	item := map[string]any{
		"result_kind": "cosa",
		"origin":      "decision-map",
		"decision_id": id,
		"file":        rel,
		"doc_path":    rel,
		"start_line":  start,
		"end_line":    end,
		"line":        start,
		"source_map":  decisionMapRel,
	}
	if !inside {
		item["content"] = ""
		return item, true
	}
	content, lineCount, truncated, readErr := readFileRange(abs, start, end, 200_000)
	if readErr != nil {
		item["content"] = ""
		item["line_count"] = 0
		return item, true
	}
	item["content"] = content
	item["line_count"] = lineCount
	if truncated {
		item["truncated"] = true
	}
	return item, true
}

func kineticCasesDir(request model.CommandRequest) string {
	if raw, ok := request.Payload["casos"].(string); ok && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	if env := strings.TrimSpace(os.Getenv("MI_LSP_CASOS")); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "repos", "mios", "home", "casos")
}

func readKineticCases(dir string) []kineticCase {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	var out []kineticCase
	lines := 0
	for _, name := range names {
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		n := 0
		for scanner.Scan() {
			if lines >= kineticCaseLineCap {
				break
			}
			lines++
			n++
			raw := strings.TrimSpace(scanner.Text())
			if raw == "" {
				continue
			}
			var c kineticCase
			if json.Unmarshal([]byte(raw), &c) != nil {
				continue
			}
			c.File = name
			c.Line = n
			out = append(out, c)
		}
		file.Close()
		if lines >= kineticCaseLineCap {
			break
		}
	}
	return out
}

func kineticCatalogDrift(cases []kineticCase, actions []kineticRow) bool {
	known := map[string]bool{}
	for _, row := range actions {
		known[foldKinetic(row.Cells["accion"])] = true
	}
	for _, c := range cases {
		if strings.TrimSpace(c.Accion) == "" {
			continue
		}
		if !known[foldKinetic(c.Accion)] {
			return true
		}
	}
	return false
}
