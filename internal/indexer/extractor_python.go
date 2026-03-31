package indexer

import (
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// extractPython parses Python source code using tree-sitter and extracts symbols.
func extractPython(repo model.WorkspaceRepo, relPath, hash string, content []byte) []model.SymbolRecord {
	bt, err := grammars.ParseFile("example.py", content)
	if err != nil {
		return nil
	}
	defer bt.Release()

	items := make([]model.SymbolRecord, 0)
	root := bt.RootNode()
	if root == nil {
		return items
	}

	lines := strings.Split(string(content), "\n")
	walkPythonNode(bt, root, "", &items, repo, relPath, hash, lines)
	return items
}

// walkPythonNode recursively walks the Python AST and extracts symbols.
func walkPythonNode(bt *gotreesitter.BoundTree, node *gotreesitter.Node, parentClass string, items *[]model.SymbolRecord, repo model.WorkspaceRepo, relPath, hash string, lines []string) {
	if node == nil {
		return
	}

	nodeType := bt.NodeType(node)

	switch nodeType {
	case "class_definition":
		handleClassDefinition(bt, node, parentClass, items, repo, relPath, hash, lines)
		return // Don't recurse further; handleClassDefinition handles recursion
	case "function_definition":
		handleFunctionDefinition(bt, node, parentClass, items, repo, relPath, hash, lines)
		return // Don't recurse further; handleFunctionDefinition handles recursion
	case "decorated_definition":
		handleDecoratedDefinition(bt, node, parentClass, items, repo, relPath, hash, lines)
		return // Don't recurse further; handleDecoratedDefinition handles recursion
	}

	// For other node types, recurse into children
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil {
			walkPythonNode(bt, child, parentClass, items, repo, relPath, hash, lines)
		}
	}
}

// handleClassDefinition extracts class definitions and recursively processes their methods.
func handleClassDefinition(bt *gotreesitter.BoundTree, node *gotreesitter.Node, parentClass string, items *[]model.SymbolRecord, repo model.WorkspaceRepo, relPath, hash string, lines []string) {
	// Extract the class name from the "name" field
	nameNode := bt.ChildByField(node, "name")
	if nameNode == nil {
		return
	}

	className := bt.NodeText(nameNode)
	startPoint := node.StartPoint()
	endPoint := node.EndPoint()
	lineIndex := int(startPoint.Row)

	docComment := ExtractDocComment(lines, lineIndex)
	searchText := BuildSearchText(className, "", docComment, "", relPath, "class")

	record := model.SymbolRecord{
		FilePath:      relPath,
		RepoID:        repo.ID,
		RepoName:      repo.Name,
		Name:          className,
		Kind:          "class",
		StartLine:     int(startPoint.Row) + 1,
		EndLine:       int(endPoint.Row) + 1,
		QualifiedName: relPath + "::" + className,
		SignatureHash: digest([]byte(relPath + ":" + className + ":class")),
		Scope:         "module",
		Language:      "python",
		FileHash:      hash,
		SearchText:    searchText,
	}
	*items = append(*items, record)

	// Get the body field and process methods/nested items within
	bodyNode := bt.ChildByField(node, "body")
	if bodyNode != nil {
		walkPythonNode(bt, bodyNode, className, items, repo, relPath, hash, lines)
	}
}

// handleFunctionDefinition extracts function or method definitions.
func handleFunctionDefinition(bt *gotreesitter.BoundTree, node *gotreesitter.Node, parentClass string, items *[]model.SymbolRecord, repo model.WorkspaceRepo, relPath, hash string, lines []string) {
	// Extract the function name from the "name" field
	nameNode := bt.ChildByField(node, "name")
	if nameNode == nil {
		return
	}

	funcName := bt.NodeText(nameNode)
	startPoint := node.StartPoint()
	endPoint := node.EndPoint()
	lineIndex := int(startPoint.Row)

	kind := "function"
	scope := "module"
	var parent string
	qualifiedName := relPath + "::" + funcName

	if parentClass != "" {
		kind = "method"
		scope = parentClass
		parent = parentClass
		qualifiedName = relPath + "::" + parentClass + "." + funcName
	}

	docComment := ExtractDocComment(lines, lineIndex)
	searchText := BuildSearchText(funcName, "", docComment, parent, relPath, kind)

	record := model.SymbolRecord{
		FilePath:      relPath,
		RepoID:        repo.ID,
		RepoName:      repo.Name,
		Name:          funcName,
		Kind:          kind,
		StartLine:     int(startPoint.Row) + 1,
		EndLine:       int(endPoint.Row) + 1,
		Parent:        parent,
		QualifiedName: qualifiedName,
		SignatureHash: digest([]byte(relPath + ":" + qualifiedName + ":" + kind)),
		Scope:         scope,
		Language:      "python",
		FileHash:      hash,
		SearchText:    searchText,
	}
	*items = append(*items, record)
}

// handleDecoratedDefinition extracts decorated class/function definitions.
// A decorated_definition wraps a class_definition or function_definition with decorators.
func handleDecoratedDefinition(bt *gotreesitter.BoundTree, node *gotreesitter.Node, parentClass string, items *[]model.SymbolRecord, repo model.WorkspaceRepo, relPath, hash string, lines []string) {
	// Find the inner class_definition or function_definition
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil {
			childType := bt.NodeType(child)
			switch childType {
			case "class_definition":
				handleClassDefinition(bt, child, parentClass, items, repo, relPath, hash, lines)
				return
			case "function_definition":
				handleFunctionDefinition(bt, child, parentClass, items, repo, relPath, hash, lines)
				return
			}
		}
	}
}
