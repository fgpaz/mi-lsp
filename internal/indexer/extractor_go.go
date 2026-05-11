package indexer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
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

func goReceiverName(receiver *ast.FieldList) string {
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
