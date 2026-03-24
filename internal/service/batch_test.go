package service

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestExecuteBatchOp_SimpleNavFind(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)

	// Seed catalog with a symbol
	seedCatalogSymbol(t, root, project, "src/Hello.cs", 2, "HelloWorld", "class")

	app := New(root, nil)
	ctx := context.Background()

	op := batchOperation{
		ID: "test-1",
		Op: "nav.find",
		Params: map[string]any{
			"pattern": "HelloWorld",
			"exact":   true,
		},
	}

	result := app.executeBatchOp(ctx, op, model.QueryOptions{Workspace: name, MaxItems: 10})

	if result.ID != "test-1" {
		t.Errorf("batch result ID = %q, want test-1", result.ID)
	}

	if result.Op != "nav.find" {
		t.Errorf("batch result Op = %q, want nav.find", result.Op)
	}

	if result.Error != "" {
		t.Errorf("batch result Error = %q, want empty", result.Error)
	}

	if !result.Envelope.Ok {
		t.Errorf("batch result Envelope.Ok = false, want true")
	}
}

func TestExecuteBatchOp_GeneratesIDFromOp(t *testing.T) {
	root, name := setupTestWorkspace(t)
	app := New(root, nil)
	ctx := context.Background()

	op := batchOperation{
		ID: "",
		Op: "nav.find",
		Params: map[string]any{
			"pattern": "test",
		},
	}

	result := app.executeBatchOp(ctx, op, model.QueryOptions{Workspace: name})

	if result.ID != "nav.find" {
		t.Errorf("batch result ID = %q, want nav.find (generated from Op)", result.ID)
	}
}

func TestExecuteBatchOp_InvalidOperation(t *testing.T) {
	root, name := setupTestWorkspace(t)
	app := New(root, nil)
	ctx := context.Background()

	op := batchOperation{
		ID: "invalid-1",
		Op: "nonexistent.operation",
		Params: map[string]any{},
	}

	result := app.executeBatchOp(ctx, op, model.QueryOptions{Workspace: name})

	if result.ID != "invalid-1" {
		t.Errorf("batch result ID = %q, want invalid-1", result.ID)
	}

	if result.Op != "nonexistent.operation" {
		t.Errorf("batch result Op = %q, want nonexistent.operation", result.Op)
	}

	if result.Error == "" {
		t.Errorf("batch result Error should not be empty for invalid operation")
	}

	if result.Envelope.Ok {
		t.Errorf("batch result Envelope.Ok = true, want false")
	}
}

func TestExecuteBatchOp_MissingWorkspace(t *testing.T) {
	root, _ := setupTestWorkspace(t)
	app := New(root, nil)
	ctx := context.Background()

	op := batchOperation{
		ID: "test-missing",
		Op: "nav.find",
		Params: map[string]any{
			"pattern": "test",
		},
	}

	// Use non-existent workspace
	result := app.executeBatchOp(ctx, op, model.QueryOptions{Workspace: "nonexistent-workspace"})

	if result.Error == "" {
		t.Error("batch result Error should not be empty for missing workspace")
	}

	if result.Envelope.Ok {
		t.Errorf("batch result Envelope.Ok = true, want false")
	}
}

func TestExecuteBatchOp_WithContext(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)
	seedCatalogSymbol(t, root, project, "src/Hello.cs", 2, "MySymbol", "class")

	app := New(root, nil)

	// Create a cancelled context to test context propagation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	op := batchOperation{
		ID: "ctx-test",
		Op: "nav.find",
		Params: map[string]any{
			"pattern": "MySymbol",
		},
	}

	result := app.executeBatchOp(ctx, op, model.QueryOptions{Workspace: name})

	// The operation should complete but the context being cancelled may affect results
	if result.ID != "ctx-test" {
		t.Errorf("batch result ID = %q, want ctx-test", result.ID)
	}
}

func TestExecuteBatchOp_ResultStructure(t *testing.T) {
	root, name := setupTestWorkspace(t)
	app := New(root, nil)
	ctx := context.Background()

	op := batchOperation{
		ID: "struct-test",
		Op: "workspace.status",
		Params: map[string]any{},
	}

	result := app.executeBatchOp(ctx, op, model.QueryOptions{Workspace: name})

	// Verify result structure
	if result.ID == "" {
		t.Error("batch result ID should not be empty")
	}

	if result.Op == "" {
		t.Error("batch result Op should not be empty")
	}

	// Envelope should always be present
	if result.Envelope.Ok == false && result.Error == "" {
		t.Error("batch result Envelope should have either Ok=true or an Error")
	}
}

func TestBatch_TableDriven(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)
	seedCatalogSymbol(t, root, project, "src/Test.cs", 1, "TestClass", "class")

	app := New(root, nil)
	ctx := context.Background()

	tests := []struct {
		name      string
		op        batchOperation
		expectErr bool
	}{
		{
			name: "valid nav.find",
			op: batchOperation{
				ID: "op1",
				Op: "nav.find",
				Params: map[string]any{
					"pattern": "TestClass",
					"exact":   true,
				},
			},
			expectErr: false,
		},
		{
			name: "invalid operation",
			op: batchOperation{
				ID: "op2",
				Op: "invalid.op",
				Params: map[string]any{},
			},
			expectErr: true,
		},
		{
			name: "missing required params",
			op: batchOperation{
				ID: "op3",
				Op: "workspace.status", // This operation doesn't require params
				Params: map[string]any{},
			},
			expectErr: false, // workspace.status doesn't require params
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := app.executeBatchOp(ctx, tt.op, model.QueryOptions{Workspace: name, MaxItems: 10})

			if tt.expectErr {
				if result.Error == "" {
					t.Errorf("%s: expected error, got none", tt.name)
				}
				if result.Envelope.Ok {
					t.Errorf("%s: expected Envelope.Ok = false, got true", tt.name)
				}
			} else {
				if result.Error != "" {
					t.Errorf("%s: expected no error, got %s", tt.name, result.Error)
				}
			}
		})
	}
}
