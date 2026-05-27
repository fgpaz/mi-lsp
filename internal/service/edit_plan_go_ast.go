package service

import (
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type goASTFunctionMatch struct {
	Decl      *ast.FuncDecl
	FileSet   *token.FileSet
	StartByte int
	EndByte   int
	BodyStart int
	BodyEnd   int
}

func applyEditPlanV2InMemory(packet *model.EditPlanRequest, targets map[string]editPlanResolvedTarget, fileStates map[string]*editPlanFileState) ([]model.EditPlanOperationResult, error) {
	results := make([]model.EditPlanOperationResult, 0, len(packet.Operations))
	for _, operation := range packet.Operations {
		target := targets[operation.TargetID]
		state := fileStates[target.RelPath]
		result := model.EditPlanOperationResult{ID: operation.ID, Kind: operation.Kind, TargetID: operation.TargetID, Path: target.RelPath, Status: "planned"}
		after, replacements, status, err := applyGoASTEdit(operation, target, state.After, packet.Constraints.RequireCleanMatch)
		if err != nil {
			return nil, fmt.Errorf("operation %s: %w", operation.ID, err)
		}
		state.After = after
		result.Status = status
		result.Replacements = replacements
		results = append(results, result)
	}
	return results, nil
}

func applyGoASTEdit(operation model.EditPlanOperation, target editPlanResolvedTarget, content []byte, requireCleanMatch bool) ([]byte, int, string, error) {
	lineEnding := detectEditPlanLineEnding(content)
	switch operation.Kind {
	case "replace_go_function":
		match, err := findGoASTFunction(target.RelPath, content, target.Target.Symbol)
		if err != nil {
			return nil, 0, "", err
		}
		replacement, err := formatGoASTFunctionSnippet(operation.Content)
		if err != nil {
			return nil, 0, "", fmt.Errorf("replacement function is invalid Go: %w", err)
		}
		edited := replaceByteRange(content, match.StartByte, match.EndByte, []byte(replacement))
		formatted, err := formatGoASTFile(edited, lineEnding)
		if err != nil {
			return nil, 0, "", fmt.Errorf("formatted Go file is invalid: %w", err)
		}
		return formatted, 1, "ok", nil
	case "replace_go_function_body":
		match, err := findGoASTFunction(target.RelPath, content, target.Target.Symbol)
		if err != nil {
			return nil, 0, "", err
		}
		if match.Decl.Body == nil {
			return nil, 0, "", errors.New("target function has no body")
		}
		body, err := formatGoASTBodySnippet(operation.Content)
		if err != nil {
			return nil, 0, "", fmt.Errorf("replacement function body is invalid Go: %w", err)
		}
		edited := replaceByteRange(content, match.BodyStart, match.BodyEnd, []byte(body))
		formatted, err := formatGoASTFile(edited, lineEnding)
		if err != nil {
			return nil, 0, "", fmt.Errorf("formatted Go file is invalid: %w", err)
		}
		return formatted, 1, "ok", nil
	case "insert_go_function_after":
		match, err := findGoASTFunction(target.RelPath, content, target.Target.Symbol)
		if err != nil {
			return nil, 0, "", err
		}
		insert, err := formatGoASTFunctionSnippet(operation.Content)
		if err != nil {
			return nil, 0, "", fmt.Errorf("inserted function is invalid Go: %w", err)
		}
		insert = "\n\n" + strings.TrimSpace(insert) + "\n"
		edited := replaceByteRange(content, match.EndByte, match.EndByte, []byte(insert))
		formatted, err := formatGoASTFile(edited, lineEnding)
		if err != nil {
			return nil, 0, "", fmt.Errorf("formatted Go file is invalid: %w", err)
		}
		return formatted, 1, "ok", nil
	case "ensure_go_import":
		after, changed, err := ensureGoASTImport(target.RelPath, content, editPlanOperationImportPath(operation), strings.TrimSpace(operation.ImportAlias), lineEnding)
		if err != nil {
			return nil, 0, "", err
		}
		return after, changed, "ok", nil
	case "remove_go_import":
		after, changed, err := removeGoASTImport(target.RelPath, content, editPlanOperationImportPath(operation), requireCleanMatch, lineEnding)
		if err != nil {
			return nil, 0, "", err
		}
		if changed == 0 {
			return after, 0, "no_match", nil
		}
		return after, changed, "ok", nil
	default:
		return nil, 0, "", fmt.Errorf("unsupported edit-plan-v2 operation kind %q", operation.Kind)
	}
}

func findGoASTFunction(relPath string, content []byte, symbol *model.EditPlanSymbol) (goASTFunctionMatch, error) {
	if symbol == nil || strings.TrimSpace(symbol.Name) == "" {
		return goASTFunctionMatch{}, errors.New("symbol.name is required")
	}
	fileSet, parsed, err := parseGoASTFile(relPath, content)
	if err != nil {
		return goASTFunctionMatch{}, err
	}
	var matches []goASTFunctionMatch
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != strings.TrimSpace(symbol.Name) {
			continue
		}
		receiver := goASTReceiverName(fn.Recv)
		kind := "function"
		if receiver != "" {
			kind = "method"
		}
		if symbol.Kind != "" && !strings.EqualFold(symbol.Kind, kind) {
			continue
		}
		if symbol.Receiver != "" && symbol.Receiver != receiver {
			continue
		}
		if symbol.Signature != "" && compactGoASTSignature(symbol.Signature) != goASTFuncSignature(fileSet, content, fn) {
			continue
		}
		start := fileSet.Position(fn.Pos()).Offset
		end := fileSet.Position(fn.End()).Offset
		bodyStart, bodyEnd := 0, 0
		if fn.Body != nil {
			bodyStart = fileSet.Position(fn.Body.Pos()).Offset
			bodyEnd = fileSet.Position(fn.Body.End()).Offset
		}
		if start < 0 || end <= start || end > len(content) {
			return goASTFunctionMatch{}, fmt.Errorf("symbol %s resolved to invalid byte range", symbol.Name)
		}
		matches = append(matches, goASTFunctionMatch{Decl: fn, FileSet: fileSet, StartByte: start, EndByte: end, BodyStart: bodyStart, BodyEnd: bodyEnd})
	}
	if len(matches) == 0 {
		return goASTFunctionMatch{}, fmt.Errorf("symbol %q not found in %s", symbol.Name, relPath)
	}
	if len(matches) > 1 {
		return goASTFunctionMatch{}, fmt.Errorf("symbol %q is ambiguous in %s; set symbol.receiver or signature", symbol.Name, relPath)
	}
	return matches[0], nil
}

func parseGoASTFile(relPath string, content []byte) (*token.FileSet, *ast.File, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, relPath, content, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse Go file %s: %w", relPath, err)
	}
	return fileSet, parsed, nil
}

func formatGoASTFunctionSnippet(content string) (string, error) {
	content = strings.TrimSpace(content)
	source := []byte("package editplan\n\n" + content + "\n")
	fileSet, parsed, err := parseGoASTFile("snippet.go", source)
	if err != nil {
		return "", err
	}
	if len(parsed.Decls) != 1 {
		return "", fmt.Errorf("expected exactly one Go function declaration, got %d declarations", len(parsed.Decls))
	}
	if _, ok := parsed.Decls[0].(*ast.FuncDecl); !ok {
		return "", errors.New("expected exactly one Go function declaration")
	}
	formatted, err := format.Source(source)
	if err != nil {
		return "", err
	}
	prefix := "package editplan\n\n"
	text := string(formatted)
	if !strings.HasPrefix(text, prefix) {
		start := fileSet.Position(parsed.Decls[0].Pos()).Offset
		if start > 0 && start < len(formatted) {
			return strings.TrimSpace(string(formatted[start:])) + "\n", nil
		}
		return "", errors.New("formatted snippet has unexpected package prefix")
	}
	return strings.TrimSpace(strings.TrimPrefix(text, prefix)) + "\n", nil
}

func formatGoASTBodySnippet(content string) (string, error) {
	source := []byte("package editplan\n\nfunc _() {\n" + strings.TrimSpace(content) + "\n}\n")
	formatted, err := format.Source(source)
	if err != nil {
		return "", err
	}
	fileSet, parsed, err := parseGoASTFile("body.go", formatted)
	if err != nil {
		return "", err
	}
	for _, decl := range parsed.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			start := fileSet.Position(fn.Body.Pos()).Offset
			end := fileSet.Position(fn.Body.End()).Offset
			if start >= 0 && end > start && end <= len(formatted) {
				return string(formatted[start:end]), nil
			}
		}
	}
	return "", errors.New("could not format Go function body")
}

func formatGoASTFile(content []byte, lineEnding string) ([]byte, error) {
	formatted, err := format.Source(content)
	if err != nil {
		return nil, err
	}
	return []byte(normalizeEditPlanLineEndings(string(formatted), lineEnding)), nil
}

func ensureGoASTImport(relPath string, content []byte, importPath string, alias string, lineEnding string) ([]byte, int, error) {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return nil, 0, errors.New("import path is required")
	}
	_, parsed, err := parseGoASTFile(relPath, content)
	if err != nil {
		return nil, 0, err
	}
	for _, spec := range parsed.Imports {
		if goASTImportPath(spec) == importPath {
			return append([]byte(nil), content...), 0, nil
		}
	}
	specText := goASTImportSpec(alias, importPath)
	var importDecl *ast.GenDecl
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.IMPORT {
			importDecl = gen
			break
		}
	}
	var edited []byte
	if importDecl == nil {
		fileSet := token.NewFileSet()
		parsedAgain, err := parser.ParseFile(fileSet, relPath, content, parser.ParseComments)
		if err != nil {
			return nil, 0, err
		}
		insertAt := lineEndIncludingNewline(content, fileSet.Position(parsedAgain.Name.End()).Offset)
		edited = replaceByteRange(content, insertAt, insertAt, []byte("\nimport "+specText+"\n"))
	} else {
		fileSet := token.NewFileSet()
		parsedAgain, err := parser.ParseFile(fileSet, relPath, content, parser.ParseComments)
		if err != nil {
			return nil, 0, err
		}
		importDecl = firstGoASTImportDecl(parsedAgain)
		if importDecl == nil {
			return nil, 0, errors.New("import declaration disappeared during parse")
		}
		if importDecl.Lparen.IsValid() {
			insertAt := fileSet.Position(importDecl.Rparen).Offset
			edited = replaceByteRange(content, insertAt, insertAt, []byte("\n\t"+specText))
		} else {
			existing := goASTImportSpecFromSpec(importDecl.Specs[0].(*ast.ImportSpec))
			replacement := []byte("import (\n\t" + existing + "\n\t" + specText + "\n)")
			start := fileSet.Position(importDecl.Pos()).Offset
			end := fileSet.Position(importDecl.End()).Offset
			edited = replaceByteRange(content, start, end, replacement)
		}
	}
	formatted, err := formatGoASTFile(edited, lineEnding)
	if err != nil {
		return nil, 0, err
	}
	return formatted, 1, nil
}

func removeGoASTImport(relPath string, content []byte, importPath string, requireCleanMatch bool, lineEnding string) ([]byte, int, error) {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return nil, 0, errors.New("import path is required")
	}
	fileSet, parsed, err := parseGoASTFile(relPath, content)
	if err != nil {
		return nil, 0, err
	}
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		for _, specNode := range gen.Specs {
			spec := specNode.(*ast.ImportSpec)
			if goASTImportPath(spec) != importPath {
				continue
			}
			var start, end int
			if len(gen.Specs) <= 1 {
				start = lineStart(content, fileSet.Position(gen.Pos()).Offset)
				end = lineEndIncludingNewline(content, fileSet.Position(gen.End()).Offset)
			} else {
				start = lineStart(content, fileSet.Position(spec.Pos()).Offset)
				end = lineEndIncludingNewline(content, fileSet.Position(spec.End()).Offset)
			}
			edited := replaceByteRange(content, start, end, nil)
			formatted, err := formatGoASTFile(edited, lineEnding)
			if err != nil {
				return nil, 0, err
			}
			return formatted, 1, nil
		}
	}
	if requireCleanMatch {
		return nil, 0, fmt.Errorf("import %q not found in %s", importPath, relPath)
	}
	return append([]byte(nil), content...), 0, nil
}

func firstGoASTImportDecl(file *ast.File) *ast.GenDecl {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.IMPORT {
			return gen
		}
	}
	return nil
}

func goASTImportPath(spec *ast.ImportSpec) string {
	if spec == nil || spec.Path == nil {
		return ""
	}
	value, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return ""
	}
	return value
}

func goASTImportSpec(alias string, importPath string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return strconv.Quote(importPath)
	}
	return alias + " " + strconv.Quote(importPath)
}

func goASTImportSpecFromSpec(spec *ast.ImportSpec) string {
	if spec == nil {
		return ""
	}
	alias := ""
	if spec.Name != nil {
		alias = spec.Name.Name
	}
	return goASTImportSpec(alias, goASTImportPath(spec))
}

func replaceByteRange(content []byte, start int, end int, replacement []byte) []byte {
	result := make([]byte, 0, len(content)-(end-start)+len(replacement))
	result = append(result, content[:start]...)
	result = append(result, replacement...)
	result = append(result, content[end:]...)
	return result
}

func lineStart(content []byte, offset int) int {
	if offset > len(content) {
		offset = len(content)
	}
	for offset > 0 && content[offset-1] != '\n' {
		offset--
	}
	return offset
}

func lineEndIncludingNewline(content []byte, offset int) int {
	if offset < 0 {
		offset = 0
	}
	for offset < len(content) && content[offset] != '\n' {
		offset++
	}
	if offset < len(content) {
		offset++
	}
	return offset
}

func goASTReceiverName(receiver *ast.FieldList) string {
	if receiver == nil || len(receiver.List) == 0 {
		return ""
	}
	expr := receiver.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	if selector, ok := expr.(*ast.SelectorExpr); ok {
		return selector.Sel.Name
	}
	return ""
}

func goASTFuncSignature(fileSet *token.FileSet, content []byte, node *ast.FuncDecl) string {
	if node == nil {
		return ""
	}
	start := fileSet.Position(node.Pos()).Offset
	end := fileSet.Position(node.Type.End()).Offset
	if start < 0 || end <= start || end > len(content) {
		return compactGoASTSignature(node.Name.Name)
	}
	return compactGoASTSignature(string(content[start:end]))
}

func compactGoASTSignature(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}
