package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func extractGo(repo model.WorkspaceRepo, relPath, hash string, content []byte) []model.SymbolRecord {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, relPath, content, parser.ParseComments)
	if err != nil {
		return nil
	}
	items := make([]model.SymbolRecord, 0)
	add := func(name, kind, signature, parent, doc string, start token.Pos, end token.Pos) {
		if strings.TrimSpace(name) == "" {
			return
		}
		startPos := fileSet.Position(start)
		endPos := fileSet.Position(end)
		if endPos.Line == 0 {
			endPos.Line = startPos.Line
		}
		qualifiedName := relPath + "::" + name
		if parent != "" {
			qualifiedName = relPath + "::" + parent + "." + name
		}
		searchText := BuildSearchText(name, signature, doc, parent, relPath, kind)
		items = append(items, model.SymbolRecord{
			FilePath:      relPath,
			RepoID:        repo.ID,
			RepoName:      repo.Name,
			Name:          name,
			Kind:          kind,
			StartLine:     startPos.Line,
			EndLine:       endPos.Line,
			Parent:        parent,
			QualifiedName: qualifiedName,
			Signature:     signature,
			SignatureHash: digest([]byte(relPath + ":" + signature + ":" + kind)),
			Scope:         goScope(name),
			Language:      "go",
			FileHash:      hash,
			SearchText:    searchText,
		})
	}
	for _, decl := range parsed.Decls {
		switch node := decl.(type) {
		case *ast.FuncDecl:
			parent := goReceiverName(node.Recv)
			kind := "function"
			if parent != "" {
				kind = "method"
			}
			add(node.Name.Name, kind, goFuncSignature(fileSet, content, node), parent, goDocText(node.Doc), node.Pos(), node.End())
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					kind := "type"
					switch typed.Type.(type) {
					case *ast.StructType:
						kind = "struct"
					case *ast.InterfaceType:
						kind = "interface"
					}
					add(typed.Name.Name, kind, goNodeText(fileSet, content, typed), "", goDocText(firstCommentGroup(typed.Doc, node.Doc)), typed.Pos(), typed.End())
				case *ast.ValueSpec:
					kind := strings.ToLower(node.Tok.String())
					for _, name := range typed.Names {
						add(name.Name, kind, goNodeText(fileSet, content, typed), "", goDocText(firstCommentGroup(typed.Doc, node.Doc)), name.Pos(), typed.End())
					}
				}
			}
		}
	}
	return items
}

func goScope(name string) string {
	if name == "" {
		return ""
	}
	if ast.IsExported(name) {
		return "public"
	}
	return "package"
}

type goReceiverInfo struct {
	name    string
	pointer bool
}

func goReceiver(receiver *ast.FieldList) goReceiverInfo {
	if receiver == nil || len(receiver.List) == 0 {
		return goReceiverInfo{}
	}
	var unwrap func(ast.Expr, bool) goReceiverInfo
	unwrap = func(expr ast.Expr, pointer bool) goReceiverInfo {
		switch x := expr.(type) {
		case *ast.ParenExpr:
			return unwrap(x.X, pointer)
		case *ast.StarExpr:
			return unwrap(x.X, true)
		case *ast.IndexExpr:
			return unwrap(x.X, pointer)
		case *ast.IndexListExpr:
			return unwrap(x.X, pointer)
		case *ast.Ident:
			return goReceiverInfo{name: x.Name, pointer: pointer}
		case *ast.SelectorExpr:
			return goReceiverInfo{name: x.Sel.Name, pointer: pointer}
		default:
			return goReceiverInfo{pointer: pointer}
		}
	}
	return unwrap(receiver.List[0].Type, false)
}

func goReceiverName(receiver *ast.FieldList) string { return goReceiver(receiver).name }

func goFuncSignature(fileSet *token.FileSet, content []byte, node *ast.FuncDecl) string {
	if node == nil {
		return ""
	}
	start := fileSet.Position(node.Pos()).Offset
	end := fileSet.Position(node.Type.End()).Offset
	if start < 0 || end <= start || end > len(content) {
		return strings.TrimSpace(node.Name.Name)
	}
	return compactGoSignature(string(content[start:end]))
}

func goNodeText(fileSet *token.FileSet, content []byte, node ast.Node) string {
	if node == nil {
		return ""
	}
	start := fileSet.Position(node.Pos()).Offset
	end := fileSet.Position(node.End()).Offset
	if start < 0 || end <= start || end > len(content) {
		return ""
	}
	return compactGoSignature(string(content[start:end]))
}

func compactGoSignature(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}

func goDocText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Text())
}

func firstCommentGroup(groups ...*ast.CommentGroup) *ast.CommentGroup {
	for _, group := range groups {
		if group != nil {
			return group
		}
	}
	return nil
}

func isGoPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".go")
}

// GoGraphObservationRequest describes one exact, repository-relative Go module selection.
type GoGraphObservationRequest struct {
	Root               string
	RepositoryIdentity string
	ProjectOrModule    string
	BuildTags          []string
	GOOS               string
	GOARCH             string
}

type goListModule struct {
	Path      string
	Dir       string
	GoMod     string
	GoVersion string
	Replace   *goListModule
}

type goListPackage struct {
	Dir        string
	ImportPath string
	Export     string
	Module     *goListModule
	GoFiles    []string
	CgoFiles   []string
	Imports    []string
	Error      *struct{ Err string } `json:"Error"`
}

type goGraphFile struct {
	rel  string
	data []byte
	file *ast.File
}

type goGraphPackage struct {
	list    goListPackage
	files   []goGraphFile
	fset    *token.FileSet
	nodes   []*goGraphDecl
	posRefs map[token.Pos]string
	pkg     *types.Package
	info    *types.Info
	owned   bool
}

type goGraphDecl struct {
	ref       string
	keyDigest model.GraphDigest
	node      ast.Node
	kind      string
	identity  string
	owner     string
	pkg       *goGraphPackage
	parent    string
	name      string
	file      *goGraphFile
	rng       model.GraphObservationRange
}

type goGraphBuilder struct {
	batch              model.GraphObservationBatch
	root               string
	moduleRoot         string
	modulePath         string
	packages           map[string]*goGraphPackage
	local              map[string]*goGraphPackage
	objectRefs         map[types.Object]string
	nodes              map[string]model.GraphObservationNode
	edges              map[string]model.GraphObservationEdge
	edgeDigests        map[string]model.GraphDigest
	evidence           []model.GraphObservationEvidence
	omissions          map[string]model.GraphObservationOmission
	unresolved         map[string]model.GraphObservationUnresolved
	evidenceOrd        map[string]int
	evidenceKeys       map[string]bool
	exports            map[string]string
	partial            bool
	importCache        map[string]*types.Package
	initOrdinals       map[string]int
	typeErrorPositions map[string][]token.Pos
	sourceDomain       []byte
}

const goGraphExtractorVersion = "go-compiler-observation-v1"

func goGraphError(code, field, message string) error {
	return &model.GraphObservationError{Code: code, Field: field, Message: message}
}

func ObserveGoGraph(ctx context.Context, req GoGraphObservationRequest) (model.GraphObservationBatch, error) {
	var empty model.GraphObservationBatch
	root, project, goos, goarch, tags, err := validateGoGraphRequest(req)
	if err != nil {
		return empty, err
	}
	req.RepositoryIdentity, _ = model.NormalizeRepositoryIdentity(req.RepositoryIdentity)
	modPath := filepath.Join(root, filepath.FromSlash(project))
	modInfo, err := os.Stat(modPath)
	if err != nil || modInfo.IsDir() {
		return empty, goGraphError("GPH_GO_MOD_MISSING", "project_or_module", "selected go.mod is missing")
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil || !goGraphInside(rootReal, mustEvalSymlinks(modPath)) {
		return empty, goGraphError("GPH_GO_MOD_OUTSIDE_ROOT", "project_or_module", "selected go.mod resolves outside root")
	}
	moduleRoot := filepath.Dir(modPath)
	moduleRootReal := mustEvalSymlinks(moduleRoot)
	modBytes, err := os.ReadFile(modPath)
	if err != nil {
		return empty, goGraphError("GPH_GO_MOD_UNREADABLE", "project_or_module", "selected go.mod cannot be read")
	}
	modulePath := goModulePath(modBytes)
	if modulePath == "" {
		return empty, goGraphError("GPH_GO_MOD_INVALID", "project_or_module", "selected go.mod has no valid module directive")
	}
	goSumPath := filepath.Join(moduleRoot, "go.sum")
	goSumBytes, sumErr := os.ReadFile(goSumPath)
	sumDigestBytes := goSumBytes
	if os.IsNotExist(sumErr) {
		sumDigestBytes = nil
	} else if sumErr != nil {
		sumDigestBytes = []byte("<go.sum unavailable>")
	}
	configDigest := goGraphConfigDigest(modBytes, sumDigestBytes, modulePath, project, nil, tags, goos, goarch, os.Getenv("CGO_ENABLED"), os.Getenv("GOFLAGS"))
	builder := &goGraphBuilder{
		root: root, moduleRoot: moduleRoot, modulePath: modulePath,
		packages: map[string]*goGraphPackage{}, local: map[string]*goGraphPackage{}, objectRefs: map[types.Object]string{},
		nodes: map[string]model.GraphObservationNode{}, edges: map[string]model.GraphObservationEdge{}, edgeDigests: map[string]model.GraphDigest{}, omissions: map[string]model.GraphObservationOmission{},
		unresolved: map[string]model.GraphObservationUnresolved{}, evidenceOrd: map[string]int{}, evidenceKeys: map[string]bool{}, exports: map[string]string{}, importCache: map[string]*types.Package{}, initOrdinals: map[string]int{}, typeErrorPositions: map[string][]token.Pos{},
		sourceDomain: []byte(fmt.Sprintf("go-graph-source-v1:%d:%s:%d:%x;", len(project), project, len(modBytes), modBytes)),
		batch:        model.GraphObservationBatch{SchemaVersion: model.GraphObservationSchemaVersion, WorkspaceIdentity: req.RepositoryIdentity, RepositoryIdentity: req.RepositoryIdentity, Backend: "go", BackendVersion: runtime.Version(), ExtractorVersion: goGraphExtractorVersion, ProjectOrModule: project, ConfigFingerprint: configDigest, Capabilities: goGraphCapabilities()},
	}
	if sumErr != nil && !os.IsNotExist(sumErr) {
		return empty, goGraphError("GPH_GO_SUM_UNREADABLE", "go_sum", "selected go.sum cannot be read")
	}
	if ctx.Err() != nil {
		builder.partial = true
		builder.addPartialOmissions(project, "cancelled")
		builder.batch.SourceFingerprint = builder.sourceFingerprint()
		return builder.finish()
	}
	listed, listErr := goGraphList(ctx, moduleRoot, tags, goos, goarch)
	if listErr != nil {
		builder.partial = true
		if errors.Is(listErr, context.Canceled) || errors.Is(listErr, context.DeadlineExceeded) {
			builder.addPartialOmissions(project, "cancelled")
			builder.batch.SourceFingerprint = builder.sourceFingerprint()
			return builder.finish()
		}
		if len(listed) == 0 {
			return empty, goGraphError("GPH_GO_LIST_UNAVAILABLE", "go_list", "go list could not produce package metadata")
		}
		builder.addPartialOmissions(project, "listing_error")
	}
	for _, p := range listed {
		if p.ImportPath != "" && p.Export != "" {
			builder.exports[p.ImportPath] = p.Export
		}
		if p.ImportPath == "" || p.Module == nil || p.Module.Path != modulePath || !goGraphResolvedInside(moduleRoot, moduleRootReal, p.Dir) || goGraphSkippedPath(p.Dir) {
			continue
		}
		gp := &goGraphPackage{list: p, fset: token.NewFileSet(), posRefs: map[token.Pos]string{}, owned: true}
		names := append(append([]string{}, p.GoFiles...), p.CgoFiles...)
		sort.Strings(names)
		for _, name := range uniqueStrings(names) {
			full := filepath.Join(p.Dir, name)
			if !goGraphResolvedInside(moduleRoot, moduleRootReal, full) || goGraphSkippedPath(full) {
				continue
			}
			rel, e := filepath.Rel(moduleRoot, full)
			if e != nil {
				builder.partial = true
				continue
			}
			rel = filepath.ToSlash(rel)
			data, e := os.ReadFile(full)
			if e != nil {
				builder.partial = true
				builder.addFileIssue(rel, "read_error")
				continue
			}
			file, e := parser.ParseFile(gp.fset, rel, data, parser.ParseComments|parser.AllErrors)
			if e != nil {
				builder.partial = true
				builder.addFileIssue(rel, "parse_error")
			}
			if file != nil {
				gp.files = append(gp.files, goGraphFile{rel: rel, data: data, file: file})
			}
		}
		builder.packages[p.ImportPath], builder.local[p.ImportPath] = gp, gp
		if p.Error != nil {
			reason := "listing_error"
			if strings.Contains(strings.ToLower(p.Error.Err), "cgo") {
				reason = "cgo_list_error"
			}
			builder.addPackageIssue(gp, reason)
		}
		if len(gp.files) == 0 {
			builder.partial = true
			builder.addPackageIssue(gp, "no_parseable_sources")
		}
	}
	if len(builder.local) == 0 {
		if !goGraphHasGoSource(moduleRoot) {
			return empty, goGraphError("GPH_GO_NO_OWNED_SOURCES", "sources", "selected module has no owned Go sources")
		}
		// Some target configurations exclude cgo-only packages from go list. Preserve
		// a valid, explicitly partial observation instead of returning an unusable batch.
		builder.partial = true
		builder.addPartialOmissions(project, "cgo_list_error")
		builder.batch.SourceFingerprint = builder.sourceFingerprint()
		return builder.finish()
	}
	importPaths := make([]string, 0, len(listed))
	for _, p := range listed {
		if p.ImportPath != "" {
			importPaths = append(importPaths, p.ImportPath)
		}
	}
	sort.Strings(importPaths)
	importPaths = uniqueStrings(importPaths)
	builder.batch.ConfigFingerprint = goGraphConfigDigest(modBytes, sumDigestBytes, modulePath, project, importPaths, tags, goos, goarch, os.Getenv("CGO_ENABLED"), os.Getenv("GOFLAGS"))
	builder.batch.SourceFingerprint = builder.sourceFingerprint()
	builder.extractDeclarations()
	builder.extractImportsAndContains()
	builder.typeCheckAll(ctx)
	builder.extractTypedRelations(ctx)
	if ctx.Err() != nil {
		builder.partial = true
		builder.addPartialOmissions(project, "cancelled")
	}
	if builder.partial {
		builder.markPartialReasons(project)
	}
	return builder.finish()
}

func goGraphCapabilities() []model.GraphObservationCapability {
	return []model.GraphObservationCapability{
		{Backend: "go", Capability: "declarations", State: model.GraphObservationStatusStable},
		{Backend: "go", Capability: "contains", State: model.GraphObservationStatusStable},
		{Backend: "go", Capability: "imports", State: model.GraphObservationStatusStable},
		{Backend: "go", Capability: "references", State: model.GraphObservationStatusStable},
		{Backend: "go", Capability: "calls", State: model.GraphObservationStatusStable},
	}
}

func validateGoGraphRequest(req GoGraphObservationRequest) (string, string, string, string, []string, error) {
	if strings.TrimSpace(req.Root) == "" {
		return "", "", "", "", nil, goGraphError("GPH_GO_ROOT_INVALID", "root", "root is required")
	}
	root, err := filepath.Abs(filepath.Clean(req.Root))
	if err != nil {
		return "", "", "", "", nil, goGraphError("GPH_GO_ROOT_INVALID", "root", "root is invalid")
	}
	project, err := goGraphRelative(req.ProjectOrModule)
	if err != nil || filepath.Base(project) != "go.mod" {
		return "", "", "", "", nil, goGraphError("GPH_GO_MOD_INVALID", "project_or_module", "project_or_module must be an exact relative go.mod path")
	}
	if !goGraphInside(root, filepath.Join(root, filepath.FromSlash(project))) {
		return "", "", "", "", nil, goGraphError("GPH_GO_MOD_OUTSIDE_ROOT", "project_or_module", "selected go.mod is outside root")
	}
	identity, err := model.NormalizeRepositoryIdentity(req.RepositoryIdentity)
	if err != nil {
		return "", "", "", "", nil, goGraphError("GPH_GO_REPOSITORY_INVALID", "repository_identity", "repository identity is invalid")
	}
	_ = identity
	validateTarget := func(value, field string) error {
		if value == "" || len(value) > 64 {
			return goGraphError("GPH_GO_TARGET_INVALID", field, "target is empty or too long")
		}
		for _, r := range value {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
				return goGraphError("GPH_GO_TARGET_INVALID", field, "target contains unsupported characters")
			}
		}
		return nil
	}
	goos, goarch := req.GOOS, req.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if err := validateTarget(goos, "GOOS"); err != nil {
		return "", "", "", "", nil, err
	}
	if err := validateTarget(goarch, "GOARCH"); err != nil {
		return "", "", "", "", nil, err
	}
	tags := make([]string, 0, len(req.BuildTags))
	for _, tag := range req.BuildTags {
		if strings.TrimSpace(tag) == "" || len(tag) > 128 || strings.ContainsAny(tag, "\r\n\t, ") {
			return "", "", "", "", nil, goGraphError("GPH_GO_TAG_INVALID", "build_tags", "build tag is invalid")
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return root, project, goos, goarch, uniqueStrings(tags), nil
}

func mustEvalSymlinks(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolved
}

func goGraphResolvedInside(lexicalRoot, resolvedRoot, path string) bool {
	return goGraphInside(lexicalRoot, path) && goGraphInside(resolvedRoot, mustEvalSymlinks(path))
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func (b *goGraphBuilder) addFileIssue(owner, reason string) {
	for _, capability := range []string{"declarations", "contains", "imports", "references", "calls"} {
		b.addOmission(owner, "file", capability, reason)
	}
}

func (b *goGraphBuilder) addPackageIssue(p *goGraphPackage, reason string) {
	owner := b.batch.ProjectOrModule
	if p != nil && len(p.files) > 0 {
		owner = p.files[0].rel
	} else if p != nil && p.list.ImportPath != "" {
		owner = p.list.ImportPath
	}
	b.addFileIssue(owner, reason)
}

func goGraphRelative(path string) (string, error) {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) {
		return "", errors.New("absolute or empty path")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("outside path")
	}
	if strings.Contains(path, "\\") && filepath.Separator == '/' {
		clean = filepath.FromSlash(strings.ReplaceAll(path, "\\", "/"))
	}
	return filepath.ToSlash(clean), nil
}

func goGraphInside(root, path string) bool {
	if path == "" {
		return false
	}
	r, err := filepath.Rel(root, path)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}
func goGraphHasGoSource(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if entry.IsDir() && goGraphSkippedPath(path) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && isGoPath(path) {
			found = true
		}
		return nil
	})
	return found
}

func goGraphSkippedPath(path string) bool {
	p := filepath.ToSlash(path)
	for _, part := range strings.Split(p, "/") {
		if part == "vendor" || part == ".git" || part == "bin" {
			return true
		}
	}
	return false
}
func goModulePath(data []byte) string {
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
func goGraphDigest(data []byte) model.GraphDigest { return model.GraphDigest(sha256.Sum256(data)) }
func goGraphConfigDigest(mod, sum []byte, modulePath, project string, imports, tags []string, goos, goarch, cgo, goflags string) model.GraphDigest {
	imports = append([]string(nil), imports...)
	sort.Strings(imports)
	imports = uniqueStrings(imports)
	tags = append([]string(nil), tags...)
	sort.Strings(tags)
	tags = uniqueStrings(tags)
	var b strings.Builder
	write := func(label string, value []byte) { fmt.Fprintf(&b, "%s:%d:%x;", label, len(value), value) }
	write("domain", []byte("go-graph-config-v1"))
	write("go.mod", mod)
	write("go.sum", sum)
	write("module", []byte(modulePath))
	write("project", []byte(project))
	write("imports", []byte(strings.Join(imports, "\x00")))
	write("runtime", []byte(runtime.Version()))
	write("GOOS", []byte(goos))
	write("GOARCH", []byte(goarch))
	write("CGO_ENABLED", []byte(cgo))
	write("GOFLAGS", []byte(goflags))
	write("tags", []byte(strings.Join(tags, "\x00")))
	return goGraphDigest([]byte(b.String()))
}
func (b *goGraphBuilder) sourceFingerprint() model.GraphDigest {
	files := map[string][]byte{}
	for _, p := range b.local {
		for _, f := range p.files {
			files[f.rel] = f.data
		}
	}
	paths := make([]string, 0, len(files))
	for rel := range files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	var out strings.Builder
	out.Write(b.sourceDomain)
	for _, rel := range paths {
		fmt.Fprintf(&out, "%d:%s:%d:%x;", len(rel), rel, len(files[rel]), files[rel])
	}
	return goGraphDigest([]byte(out.String()))
}

func goGraphList(ctx context.Context, dir string, tags []string, goos, goarch string) ([]goListPackage, error) {
	args := []string{"list", "-deps", "-export", "-compiled", "-json", "-e", "-mod=readonly"}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, "./...")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	env := os.Environ()
	env = append(env, "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOOS="+goos, "GOARCH="+goarch)
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var packages []goListPackage
	dec := json.NewDecoder(stdout)
	for {
		var p goListPackage
		e := dec.Decode(&p)
		if e == io.EOF {
			break
		}
		if e != nil {
			_ = cmd.Wait()
			return packages, e
		}
		packages = append(packages, p)
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return packages, ctx.Err()
	}
	return packages, waitErr
}

func (b *goGraphBuilder) addNode(p *goGraphPackage, file *goGraphFile, node ast.Node, kind, identity, display, parent string, claim, resolution string) string {
	if file == nil || node == nil || identity == "" {
		return ""
	}
	start, end := p.fset.Position(node.Pos()), p.fset.Position(node.End())
	if start.Line < 1 || end.Line < start.Line {
		return ""
	}
	owner := file.rel
	key := model.NodeKeyFields{RepositoryIdentity: b.batch.RepositoryIdentity, BackendType: "go", Language: "go", ProjectOrModule: b.batch.ProjectOrModule, OwnerPath: owner, SymbolKind: kind, SemanticIdentity: identity}
	digest, err := model.HashNodeKey(key)
	if err != nil {
		return ""
	}
	ref := model.NodeRID(digest)
	if existing, exists := b.nodes[ref]; exists {
		if existing.Key.OwnerPath != owner {
			b.partial = true
			b.addOmission(owner, kind, "declarations", "semantic_identity_owner_collision")
			return ""
		}
		return ref
	}
	rng := model.GraphObservationRange{StartLine: start.Line, StartColumn: start.Column, EndLine: end.Line, EndColumn: end.Column}
	n := model.GraphObservationNode{Ref: ref, Key: key, DisplayName: display, SourceDigest: goGraphDigest(file.data), ClaimStatus: claim, Resolution: resolution}
	b.nodes[ref] = n
	d := &goGraphDecl{ref: ref, keyDigest: digest, node: node, kind: kind, identity: identity, owner: owner, parent: parent, name: display, pkg: p, file: file, rng: rng}
	p.nodes = append(p.nodes, d)
	p.posRefs[node.Pos()] = ref
	obs := goGraphDigest([]byte(fmt.Sprintf("node-claim-v1\x00%s\x00%s\x00%s\x00%d:%d-%d:%d\x00%s", ref, identity, owner, rng.StartLine, rng.StartColumn, rng.EndLine, rng.EndColumn, n.SourceDigest.String())))
	b.addEvidence(ref, "", owner, rng, n.SourceDigest, obs, "declaration", claim)
	return ref
}
func (b *goGraphBuilder) addEvidence(nodeRef, edgeRef, owner string, rng model.GraphObservationRange, source, observed model.GraphDigest, claim, status string) {
	key := nodeRef
	subject := source
	locationKey := fmt.Sprintf("%s|%s|%d:%d-%d:%d|%s|%s|%s", nodeRef+edgeRef, owner, rng.StartLine, rng.StartColumn, rng.EndLine, rng.EndColumn, source.String(), claim, observed.String())
	if b.evidenceKeys[locationKey] {
		return
	}
	b.evidenceKeys[locationKey] = true
	if edgeRef != "" {
		key = edgeRef
		subject = b.edgeDigests[edgeRef]
	}
	ordinal := b.evidenceOrd[key]
	b.evidenceOrd[key] = ordinal + 1
	subjectDigest := subject
	if nodeRef != "" {
		subjectDigest = b.nodes[nodeRef].SourceDigest
	}
	d := model.EvidenceKey(subjectDigest, observed, ordinal)
	b.evidence = append(b.evidence, model.GraphObservationEvidence{Ref: model.EvidenceRID(d), NodeRef: nodeRef, EdgeRef: edgeRef, SourceURI: owner, Range: &rng, Backend: "go", ExtractorVersion: goGraphExtractorVersion, SourceDigest: source, ObservedDigest: observed, ClaimKind: claim, Status: status})
}
func goGraphEdgeClaimDigest(from, to, relation, owner string, rng model.GraphObservationRange, source model.GraphDigest) model.GraphDigest {
	return goGraphDigest([]byte(fmt.Sprintf("edge-claim-v1\x00%s\x00%s\x00%s\x00%s\x00%d:%d-%d:%d\x00%s", from, to, relation, owner, rng.StartLine, rng.StartColumn, rng.EndLine, rng.EndColumn, source.String())))
}

func (b *goGraphBuilder) addEdge(from, to, relation, owner string, rng model.GraphObservationRange, source model.GraphDigest, status, resolution string) {
	if from == "" || to == "" {
		return
	}
	fromNode, fok := b.nodes[from]
	toNode, tok := b.nodes[to]
	if !fok || !tok {
		return
	}
	fromDigest, _ := model.HashNodeKey(fromNode.Key)
	toDigest, _ := model.HashNodeKey(toNode.Key)
	keyDigest := model.EdgeKey(fromDigest, toDigest, relation, "symbol")
	ref := model.EdgeRID(keyDigest)
	if old, ok := b.edges[ref]; ok {
		if old.OwnerPath != owner || old.SourceDigest != source {
			b.addOmission(owner, "file", relation, "additional_owner_evidence")
			return
		}
		b.addEvidence("", ref, owner, rng, source, goGraphEdgeClaimDigest(from, to, relation, owner, rng, source), relation, status)
		return
	}
	e := model.GraphObservationEdge{Ref: ref, FromRef: from, ToRef: to, Relation: relation, Scope: "symbol", Status: status, OwnerPath: owner, Backend: "go", Resolution: resolution, SourceDigest: source}
	b.edges[ref] = e
	b.edgeDigests[ref] = keyDigest
	b.addEvidence("", ref, owner, rng, source, goGraphEdgeClaimDigest(from, to, relation, owner, rng, source), relation, status)
}
func (b *goGraphBuilder) addOmission(owner, subject, capability, reason string) {
	if owner == "" {
		owner = b.batch.ProjectOrModule
	}
	key := owner + "\x00" + subject + "\x00" + capability + "\x00" + reason
	if _, ok := b.omissions[key]; ok {
		return
	}
	d := goGraphDigest([]byte("omission\x00" + key))
	b.omissions[key] = model.GraphObservationOmission{Ref: "milsp:gph-omission:v1:" + d.String(), OwnerPath: owner, SubjectKind: subject, Backend: "go", Capability: capability, ReasonCode: reason, RecoveryHintCode: "retry"}
}
func (b *goGraphBuilder) addUnresolved(owner, subject, capability, reason string, source *model.GraphDigest) {
	key := owner + "\x00" + subject + "\x00" + capability + "\x00" + reason
	if _, ok := b.unresolved[key]; ok {
		return
	}
	d := goGraphDigest([]byte("unresolved\x00" + key))
	b.unresolved[key] = model.GraphObservationUnresolved{Ref: model.UnresolvedRID(d), OwnerPath: owner, SubjectKind: subject, Capability: capability, SelectorDigest: d, ReasonCode: reason, Backend: "go", SourceDigest: source, RecoveryHintCode: "retry"}
}

func goGraphPackageSourceDigest(p *goGraphPackage) model.GraphDigest {
	var out strings.Builder
	for _, f := range p.files {
		fmt.Fprintf(&out, "%d:%s:%d:%x;", len(f.rel), f.rel, len(f.data), f.data)
	}
	return goGraphDigest([]byte("go-package-source-v1:" + out.String()))
}

func (b *goGraphBuilder) addPackageNode(p *goGraphPackage) string {
	if p == nil || len(p.files) == 0 || p.files[0].file == nil {
		return ""
	}
	file := &p.files[0]
	owner := "@package/" + p.list.ImportPath
	identity := "pkg:" + p.list.ImportPath
	key := model.NodeKeyFields{RepositoryIdentity: b.batch.RepositoryIdentity, BackendType: "go", Language: "go", ProjectOrModule: b.batch.ProjectOrModule, OwnerPath: owner, SymbolKind: "package", SemanticIdentity: identity}
	digest, err := model.HashNodeKey(key)
	if err != nil {
		return ""
	}
	ref := model.NodeRID(digest)
	if existing, ok := b.nodes[ref]; ok {
		return existing.Ref
	}
	rng := goGraphRange(p.fset, file.file.Pos(), file.file.End())
	if rng.StartLine < 1 || rng.StartColumn < 1 || rng.EndLine < rng.StartLine || rng.EndColumn < 1 {
		return ""
	}
	source := goGraphPackageSourceDigest(p)
	n := model.GraphObservationNode{Ref: ref, Key: key, DisplayName: p.list.ImportPath, SourceDigest: source, ClaimStatus: model.GraphRecordExtracted, Resolution: "go/ast"}
	b.nodes[ref] = n
	d := &goGraphDecl{ref: ref, keyDigest: digest, node: file.file, kind: "package", identity: identity, owner: owner, name: p.list.ImportPath, pkg: p, file: file, rng: rng}
	p.nodes = append(p.nodes, d)
	obs := goGraphDigest([]byte(fmt.Sprintf("package-node-claim-v1\\x00%s\\x00%s\\x00%d:%d-%d:%d\\x00%s", ref, owner, rng.StartLine, rng.StartColumn, rng.EndLine, rng.EndColumn, source.String())))
	b.addEvidence(ref, "", owner, rng, source, obs, "declaration", model.GraphRecordExtracted)
	return ref
}

func (b *goGraphBuilder) extractDeclarations() {
	paths := make([]string, 0, len(b.local))
	for path := range b.local {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, importPath := range paths {
		p := b.local[importPath]
		sort.Slice(p.files, func(i, j int) bool { return p.files[i].rel < p.files[j].rel })
		if len(p.files) == 0 {
			continue
		}
		b.addPackageNode(p)
		for i := range p.files {
			b.extractFileDeclarations(p, &p.files[i])
		}
	}
}
func (b *goGraphBuilder) extractFileDeclarations(p *goGraphPackage, f *goGraphFile) {
	for _, decl := range f.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			receiver := goReceiver(d.Recv)
			parent := receiver.name
			kind := "function"
			if parent != "" {
				kind = "method"
			}
			ordinal := ""
			if d.Name.Name == "init" && parent == "" {
				key := p.list.ImportPath + ":" + f.rel + ":init"
				ordinal = fmt.Sprintf(":%d", b.initOrdinals[key])
				b.initOrdinals[key]++
			}
			identity := "func:" + p.list.ImportPath + ":" + d.Name.Name + ordinal
			if kind == "method" {
				marker := "value"
				if receiver.pointer {
					marker = "pointer"
				}
				identity = "method:" + p.list.ImportPath + ":" + parent + ":" + marker + ":" + d.Name.Name
			}
			ref := b.addNode(p, f, d, kind, identity, d.Name.Name, parent, model.GraphRecordExtracted, "go/ast")
			if ref != "" {
				p.posRefs[d.Name.Pos()] = ref
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					identity := "type:" + p.list.ImportPath + ":" + typed.Name.Name
					ref := b.addNode(p, f, typed, "type", identity, typed.Name.Name, "", model.GraphRecordExtracted, "go/ast")
					if ref != "" {
						p.posRefs[typed.Name.Pos()] = ref
						b.extractTypeFields(p, f, typed, ref)
					}
				case *ast.ValueSpec:
					for _, name := range typed.Names {
						if name.Name == "_" {
							continue
						}
						identity := "field:" + p.list.ImportPath + ":" + name.Name
						ref := b.addNode(p, f, typed, "field", identity, name.Name, "", model.GraphRecordExtracted, "go/ast")
						if ref != "" {
							p.posRefs[name.Pos()] = ref
						}
					}
				}
			}
		}
	}
}
func (b *goGraphBuilder) extractTypeFields(p *goGraphPackage, f *goGraphFile, ts *ast.TypeSpec, typeRef string) {
	var fields *ast.FieldList
	interfaceFields := false
	switch t := ts.Type.(type) {
	case *ast.StructType:
		fields = t.Fields
	case *ast.InterfaceType:
		fields, interfaceFields = t.Methods, true
	}
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		names := field.Names
		if len(names) == 0 {
			b.addOmission(f.rel, "field", "declarations", "embedded_field_unsupported")
			continue
		}
		for _, name := range names {
			if name.Name == "_" {
				continue
			}
			kind := "field"
			prefix := "field"
			if interfaceFields {
				kind, prefix = "method", "method"
			}
			identity := prefix + ":" + p.list.ImportPath + ":" + ts.Name.Name + ":" + name.Name
			ref := b.addNode(p, f, field, kind, identity, name.Name, ts.Name.Name, model.GraphRecordExtracted, "go/ast")
			if ref != "" {
				p.posRefs[name.Pos()] = ref
				b.addEdge(typeRef, ref, "contains", f.rel, goGraphRange(p.fset, field.Pos(), field.End()), goGraphDigest(f.data), model.GraphRecordExtracted, "go/ast")
			}
		}
	}
}

func goGraphReceiverText(fs *token.FileSet, data []byte, recv *ast.FieldList) string {
	return goNodeText(fs, data, recv)
}
func goGraphRange(fs *token.FileSet, start, end token.Pos) model.GraphObservationRange {
	a, z := fs.Position(start), fs.Position(end)
	return model.GraphObservationRange{StartLine: a.Line, StartColumn: a.Column, EndLine: z.Line, EndColumn: z.Column}
}

func (b *goGraphBuilder) extractImportsAndContains() {
	paths := make([]string, 0, len(b.local))
	for path := range b.local {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		p := b.local[path]
		if len(p.files) == 0 {
			continue
		}
		pkgRef := b.findDecl(p, "package", "pkg:"+p.list.ImportPath)
		for _, f := range p.files {
			for _, spec := range f.file.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil || importPath == "" {
					b.addOmission(f.rel, "package", "imports", "import_path_invalid")
					continue
				}
				if local := b.local[importPath]; local != nil {
					to := b.findDecl(local, "package", "pkg:"+importPath)
					if to != "" {
						b.addEdge(pkgRef, to, "imports", f.rel, goGraphRange(p.fset, spec.Pos(), spec.End()), goGraphDigest(f.data), model.GraphRecordExtracted, "go/ast")
					}
				} else {
					b.addOmission(f.rel, "package", "imports", "external_target")
				}
			}
		}
		for _, d := range p.nodes {
			if d.kind == "package" {
				continue
			}
			if d.parent == "" || d.kind == "type" {
				b.addEdge(pkgRef, d.ref, "contains", d.owner, d.rng, goGraphDigest(d.file.data), model.GraphRecordExtracted, "go/ast")
			}
			if d.parent != "" {
				if parent := b.findTypeDecl(p, d.parent); parent != "" {
					b.addEdge(parent, d.ref, "contains", d.owner, d.rng, goGraphDigest(d.file.data), model.GraphRecordExtracted, "go/ast")
				}
			}
		}
	}
}
func (b *goGraphBuilder) findDecl(p *goGraphPackage, kind, identity string) string {
	for _, d := range p.nodes {
		if d.kind == kind && d.identity == identity {
			return d.ref
		}
	}
	return ""
}
func (b *goGraphBuilder) findTypeDecl(p *goGraphPackage, name string) string {
	for _, d := range p.nodes {
		if d.kind == "type" && d.name == name {
			return d.ref
		}
	}
	return ""
}

func (b *goGraphBuilder) typeCheckAll(ctx context.Context) {
	paths := make([]string, 0, len(b.local))
	for path := range b.local {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	checked, checking := map[string]bool{}, map[string]bool{}
	lookup := func(requested string) (io.ReadCloser, error) {
		exportPath := b.exports[requested]
		if exportPath == "" {
			return nil, fmt.Errorf("export metadata unavailable for %s", requested)
		}
		return os.Open(exportPath)
	}
	exportImporter := importer.ForCompiler(token.NewFileSet(), "gc", lookup)
	fallbackImporter := importer.Default()
	var check func(string)
	check = func(path string) {
		if checked[path] || checking[path] {
			return
		}
		p := b.local[path]
		if p == nil {
			return
		}
		checking[path] = true
		for _, imp := range p.list.Imports {
			if b.local[imp] != nil {
				check(imp)
			}
		}
		if len(p.files) == 0 {
			b.partial = true
			b.addPackageIssue(p, "type_check_no_sources")
			checked[path] = true
			delete(checking, path)
			return
		}
		if ctx.Err() != nil {
			b.partial = true
			b.addPackageIssue(p, "cancelled")
			delete(checking, path)
			return
		}
		info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Types: map[ast.Expr]types.TypeAndValue{}}
		typeError := false
		cfg := &types.Config{Importer: goGraphImporter{local: b.local, exports: b.exports, export: exportImporter, fallback: fallbackImporter, cache: b.importCache}, Error: func(err error) {
			typeError = true
			b.partial = true
			if e, ok := err.(types.Error); ok {
				b.typeErrorPositions[path] = append(b.typeErrorPositions[path], e.Pos)
			} else if e, ok := err.(*types.Error); ok && e != nil {
				b.typeErrorPositions[path] = append(b.typeErrorPositions[path], e.Pos)
			}
		}}
		files := make([]*ast.File, 0, len(p.files))
		for i := range p.files {
			files = append(files, p.files[i].file)
		}
		pkg, err := cfg.Check(path, p.fset, files, info)
		p.pkg, p.info = pkg, info
		if err != nil || typeError {
			b.partial = true
			b.addPackageIssue(p, "type_check_error")
		}
		if pkg != nil {
			for _, obj := range info.Defs {
				if obj != nil {
					if ref := p.posRefs[obj.Pos()]; ref != "" {
						b.objectRefs[obj] = ref
					}
				}
			}
		}
		checked[path] = true
		delete(checking, path)
	}
	for _, path := range paths {
		check(path)
	}
}

type goGraphImporter struct {
	local    map[string]*goGraphPackage
	exports  map[string]string
	export   types.Importer
	fallback types.Importer
	cache    map[string]*types.Package
}

func (i goGraphImporter) Import(path string) (*types.Package, error) {
	if p := i.local[path]; p != nil && p.pkg != nil {
		return p.pkg, nil
	}
	if i.cache != nil {
		if pkg := i.cache[path]; pkg != nil {
			return pkg, nil
		}
	}
	var pkg *types.Package
	var err error
	if i.export != nil && i.exports[path] != "" {
		pkg, err = i.export.Import(path)
	}
	if pkg == nil && err == nil {
		pkg, err = i.fallback.Import(path)
	}
	if err == nil && pkg != nil && i.cache != nil {
		i.cache[path] = pkg
	}
	return pkg, err
}

func (b *goGraphBuilder) targetSourceDigest(p *goGraphPackage, pos token.Pos) *model.GraphDigest {
	if p == nil {
		return nil
	}
	for i := range p.files {
		f := &p.files[i]
		if f.file.Pos() <= pos && pos <= f.file.End() {
			d := goGraphDigest(f.data)
			return &d
		}
	}
	return nil
}

func goGraphPackageScopeObject(p *goGraphPackage, obj types.Object) bool {
	return p != nil && p.pkg != nil && obj != nil && obj.Parent() == p.pkg.Scope()
}

func goGraphTopLevelFieldAt(p *goGraphPackage, pos token.Pos) bool {
	if p == nil {
		return false
	}
	for _, file := range p.files {
		for _, decl := range file.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				var fields *ast.FieldList
				switch typ := typeSpec.Type.(type) {
				case *ast.StructType:
					fields = typ.Fields
				case *ast.InterfaceType:
					fields = typ.Methods
				}
				if fields == nil {
					continue
				}
				for _, field := range fields.List {
					if len(field.Names) > 0 {
						for _, name := range field.Names {
							if name.Pos() == pos {
								return true
							}
						}
					} else if field.Type.Pos() <= pos && pos <= field.Type.End() {
						return true
					}
				}
			}
		}
	}
	return false
}

func goGraphTopLevelFuncAt(p *goGraphPackage, pos token.Pos) bool {
	if p == nil {
		return false
	}
	for _, file := range p.files {
		for _, decl := range file.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Name != nil && fn.Name.Pos() == pos {
				return true
			}
		}
	}
	return false
}

func goGraphLocalTargetEligible(p *goGraphPackage, obj types.Object) bool {
	switch x := obj.(type) {
	case *types.Func:
		return goGraphTopLevelFuncAt(p, obj.Pos())
	case *types.TypeName, *types.Const:
		return goGraphPackageScopeObject(p, obj)
	case *types.Var:
		if x.IsField() {
			return goGraphTopLevelFieldAt(p, obj.Pos())
		}
		return goGraphPackageScopeObject(p, obj)
	default:
		return false
	}
}

func (b *goGraphBuilder) extractTypedRelations(ctx context.Context) {
	paths := make([]string, 0, len(b.local))
	for path := range b.local {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		p := b.local[path]
		if p.info == nil {
			continue
		}
		for i := range p.files {
			f := &p.files[i]
			if ctx.Err() != nil {
				b.partial = true
				b.addFileIssue(f.rel, "cancelled")
				return
			}
			hasTypeErrorAt := func(pos token.Pos) bool {
				for _, candidate := range b.typeErrorPositions[p.list.ImportPath] {
					if candidate == pos {
						return true
					}
				}
				return false
			}
			resolve := func(obj types.Object, capability string, owner string, pos token.Pos) string {
				subject := "package"
				if node, ok := b.nodes[owner]; ok {
					subject = node.Key.SymbolKind
				}
				if obj == nil {
					if hasTypeErrorAt(pos) {
						b.partial = true
						source := goGraphDigest(f.data)
						b.addUnresolved(f.rel, subject, capability, "target_endpoint_missing", &source)
					} else {
						b.addOmission(f.rel, subject, capability, "unsupported_symbol_kind")
					}
					return ""
				}
				if ref := b.objectRefs[obj]; ref != "" {
					return ref
				}
				if typeName, ok := obj.(*types.TypeName); ok {
					if _, typeParam := typeName.Type().(*types.TypeParam); typeParam {
						b.addOmission(f.rel, subject, capability, "unsupported_symbol_kind")
						return ""
					}
				}
				if obj.Pkg() != nil {
					if targetPkg := b.local[obj.Pkg().Path()]; targetPkg != nil {
						if ref := targetPkg.posRefs[obj.Pos()]; ref != "" {
							return ref
						}
						if goGraphLocalTargetEligible(targetPkg, obj) {
							b.partial = true
							b.addUnresolved(f.rel, subject, capability, "local_target_missing_ref", b.targetSourceDigest(targetPkg, obj.Pos()))
						} else {
							b.addOmission(f.rel, subject, capability, "unsupported_symbol_kind")
						}
						return ""
					}
					b.addOmission(f.rel, subject, capability, "external_target")
					return ""
				}
				switch obj.(type) {
				case *types.Builtin:
					b.addOmission(f.rel, subject, capability, "unsupported_symbol_kind")
				default:
					b.addOmission(f.rel, subject, capability, "unsupported_symbol_kind")
				}
				return ""
			}
			ast.Inspect(f.file, func(n ast.Node) bool {
				if n == nil {
					return true
				}
				owner := b.ownerFor(p, n.Pos())
				if owner == "" {
					b.partial = true
					b.addUnresolved(f.rel, "package", "references", "owner_endpoint_missing", b.targetSourceDigest(p, n.Pos()))
					return true
				}
				switch x := n.(type) {
				case *ast.Ident:
					if p.posRefs[x.Pos()] != "" {
						return true
					}
					if target := resolve(p.info.Uses[x], "references", owner, x.Pos()); target != "" {
						b.addEdge(owner, target, "references", f.rel, goGraphRange(p.fset, x.Pos(), x.End()), goGraphDigest(f.data), model.GraphRecordExact, "go/types")
					}
				case *ast.SelectorExpr:
					var obj types.Object
					if sel := p.info.Selections[x]; sel != nil {
						obj = sel.Obj()
					} else {
						obj = p.info.Uses[x.Sel]
					}
					if target := resolve(obj, "references", owner, x.Sel.Pos()); target != "" {
						b.addEdge(owner, target, "references", f.rel, goGraphRange(p.fset, x.Sel.Pos(), x.Sel.End()), goGraphDigest(f.data), model.GraphRecordExact, "go/types")
					}
				case *ast.CallExpr:
					fun := x.Fun
					for {
						switch y := fun.(type) {
						case *ast.IndexExpr:
							fun = y.X
						case *ast.IndexListExpr:
							fun = y.X
						default:
							goto callTarget
						}
					}
				callTarget:
					var obj types.Object
					switch y := fun.(type) {
					case *ast.Ident:
						obj = p.info.Uses[y]
					case *ast.SelectorExpr:
						if sel := p.info.Selections[y]; sel != nil {
							obj = sel.Obj()
						} else {
							obj = p.info.Uses[y.Sel]
						}
					}
					if target := resolve(obj, "calls", owner, x.Pos()); target != "" {
						b.addEdge(owner, target, "calls", f.rel, goGraphRange(p.fset, x.Pos(), x.End()), goGraphDigest(f.data), model.GraphRecordExact, "go/types")
					}
				}
				return true
			})
		}
	}
}

func (b *goGraphBuilder) ownerFor(p *goGraphPackage, pos token.Pos) string {
	best := ""
	bestSpan := int64(1 << 62)
	for _, d := range p.nodes {
		if d.kind == "package" {
			continue
		}
		if d.node.Pos() <= pos && pos <= d.node.End() {
			span := int64(d.node.End() - d.node.Pos())
			if span < bestSpan {
				best, bestSpan = d.ref, span
			}
		}
	}
	if best != "" {
		return best
	}
	return b.findDecl(p, "package", "pkg:"+p.list.ImportPath)
}

func (b *goGraphBuilder) addPartialOmissions(owner, reason string) {
	for _, c := range []string{"declarations", "contains", "imports", "references", "calls"} {
		b.addOmission(owner, "file", c, reason)
	}
}
func (b *goGraphBuilder) markPartialReasons(owner string) {
	if len(b.omissions) == 0 {
		b.addPartialOmissions(owner, "partial")
	}
}
func (b *goGraphBuilder) finish() (model.GraphObservationBatch, error) {
	b.batch.Completeness = model.GraphCompletenessComplete
	if b.partial {
		b.batch.Completeness = model.GraphCompletenessPartial
	}
	b.batch.Nodes = make([]model.GraphObservationNode, 0, len(b.nodes))
	for _, n := range b.nodes {
		b.batch.Nodes = append(b.batch.Nodes, n)
	}
	b.batch.Edges = make([]model.GraphObservationEdge, 0, len(b.edges))
	for _, e := range b.edges {
		b.batch.Edges = append(b.batch.Edges, e)
	}
	b.batch.Evidence = append([]model.GraphObservationEvidence(nil), b.evidence...)
	for _, o := range b.omissions {
		b.batch.Omissions = append(b.batch.Omissions, o)
	}
	for _, u := range b.unresolved {
		b.batch.Unresolved = append(b.batch.Unresolved, u)
	}
	b.batch.Coverage = make([]model.GraphObservationCoverage, 0, 5)
	for _, cap := range []string{"declarations", "contains", "imports", "references", "calls"} {
		observed := 0
		switch cap {
		case "declarations":
			observed = len(b.batch.Nodes)
		default:
			for _, e := range b.batch.Edges {
				if e.Relation == cap {
					observed++
				}
			}
		}
		unresolved, omitted := 0, 0
		for _, u := range b.batch.Unresolved {
			if u.Capability == cap {
				unresolved++
			}
		}
		for _, o := range b.batch.Omissions {
			if o.Capability == cap {
				omitted++
			}
		}
		b.batch.Coverage = append(b.batch.Coverage, model.GraphObservationCoverage{Backend: "go", Capability: cap, Eligible: observed + unresolved + omitted, Observed: observed, Unresolved: unresolved, Omitted: omitted})
	}
	if b.batch.SourceFingerprint == (model.GraphDigest{}) {
		b.batch.SourceFingerprint = b.sourceFingerprint()
	}
	if err := model.SealGraphObservationBatch(&b.batch); err != nil {
		return model.GraphObservationBatch{}, err
	}
	return b.batch, nil
}
