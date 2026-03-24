package indexer

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestExtractPython_Classes(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "test", Name: "test-repo"}
	content := []byte(`
class Foo:
    def bar(self):
        pass

class Baz:
    def qux(self):
        pass
`)

	items := extractPython(repo, "example.py", "testhash", content)

	if len(items) < 4 {
		t.Errorf("expected at least 4 symbols, got %d", len(items))
	}

	// Check for Foo class
	found := false
	for _, item := range items {
		if item.Name == "Foo" && item.Kind == "class" {
			found = true
			if item.Scope != "module" {
				t.Errorf("Foo class should have scope 'module', got %q", item.Scope)
			}
			break
		}
	}
	if !found {
		t.Error("Foo class not found")
	}

	// Check for bar method
	found = false
	for _, item := range items {
		if item.Name == "bar" && item.Kind == "method" {
			found = true
			if item.Parent != "Foo" {
				t.Errorf("bar method parent should be 'Foo', got %q", item.Parent)
			}
			if item.Scope != "Foo" {
				t.Errorf("bar method scope should be 'Foo', got %q", item.Scope)
			}
			break
		}
	}
	if !found {
		t.Error("bar method not found")
	}
}

func TestExtractPython_Functions(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "test", Name: "test-repo"}
	content := []byte(`
def hello():
    pass

def world():
    pass

class MyClass:
    def method(self):
        pass
`)

	items := extractPython(repo, "example.py", "testhash", content)

	// Check for module-level functions
	helloFound := false
	worldFound := false
	methodFound := false

	for _, item := range items {
		if item.Name == "hello" && item.Kind == "function" {
			helloFound = true
			if item.Parent != "" {
				t.Errorf("hello function should have no parent, got %q", item.Parent)
			}
			if item.Scope != "module" {
				t.Errorf("hello function scope should be 'module', got %q", item.Scope)
			}
		}
		if item.Name == "world" && item.Kind == "function" {
			worldFound = true
		}
		if item.Name == "method" && item.Kind == "method" {
			methodFound = true
			if item.Parent != "MyClass" {
				t.Errorf("method parent should be 'MyClass', got %q", item.Parent)
			}
		}
	}

	if !helloFound {
		t.Error("hello function not found")
	}
	if !worldFound {
		t.Error("world function not found")
	}
	if !methodFound {
		t.Error("method not found")
	}
}

func TestExtractPython_AsyncDef(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "test", Name: "test-repo"}
	content := []byte(`
async def fetch():
    pass

class Service:
    async def connect(self):
        pass
`)

	items := extractPython(repo, "example.py", "testhash", content)

	// Check for async function
	found := false
	for _, item := range items {
		if item.Name == "fetch" && item.Kind == "function" {
			found = true
			break
		}
	}
	if !found {
		t.Error("fetch async function not found")
	}

	// Check for async method
	found = false
	for _, item := range items {
		if item.Name == "connect" && item.Kind == "method" {
			found = true
			if item.Parent != "Service" {
				t.Errorf("connect method parent should be 'Service', got %q", item.Parent)
			}
			break
		}
	}
	if !found {
		t.Error("connect async method not found")
	}
}

func TestExtractPython_Decorators(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "test", Name: "test-repo"}
	content := []byte(`
@property
def value(self):
    pass

class MyClass:
    @staticmethod
    def static_method():
        pass

    @classmethod
    def class_method(cls):
        pass
`)

	items := extractPython(repo, "example.py", "testhash", content)

	// Check for decorated function/method exists
	// The decorator itself is in the decorated_definition wrapper,
	// but we extract the inner definition (the function or method)
	found := false
	for _, item := range items {
		if item.Name == "value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("value decorated function not found")
	}

	// Check for static_method
	found = false
	for _, item := range items {
		if item.Name == "static_method" && item.Kind == "method" {
			found = true
			break
		}
	}
	if !found {
		t.Error("static_method not found")
	}

	// Check for class_method
	found = false
	for _, item := range items {
		if item.Name == "class_method" && item.Kind == "method" {
			found = true
			break
		}
	}
	if !found {
		t.Error("class_method not found")
	}
}

func TestExtractPython_Scope(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "test", Name: "test-repo"}
	content := []byte(`
def module_function():
    pass

class MyClass:
    def instance_method(self):
        pass
`)

	items := extractPython(repo, "example.py", "testhash", content)

	// Verify module_function has module scope
	for _, item := range items {
		if item.Name == "module_function" {
			if item.Scope != "module" {
				t.Errorf("module_function scope should be 'module', got %q", item.Scope)
			}
			if item.Kind != "function" {
				t.Errorf("module_function kind should be 'function', got %q", item.Kind)
			}
			return
		}
	}
	t.Error("module_function not found")
}

func TestExtractPython_EmptyFile(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "test", Name: "test-repo"}
	content := []byte(``)

	items := extractPython(repo, "empty.py", "testhash", content)

	// Empty file should return empty or nil
	if items != nil && len(items) > 0 {
		t.Errorf("empty file should return no symbols, got %d", len(items))
	}
}

func TestExtractPython_InvalidSyntax(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "test", Name: "test-repo"}
	// Intentionally malformed Python
	content := []byte(`
class Foo
    def bar(
`)

	items := extractPython(repo, "invalid.py", "testhash", content)

	// Should gracefully handle invalid syntax (tree-sitter is lenient)
	// or return nil on parse error
	if items == nil {
		t.Log("Invalid syntax returned nil (graceful degradation)")
		return
	}
	t.Logf("Invalid syntax returned %d items", len(items))
}
