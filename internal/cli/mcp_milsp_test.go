package cli

import (
	"encoding/json"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/mcp"
)

func TestMCPMilspBuildArgv(t *testing.T) {
	got, err := mcp.BuildArgv("milsp", map[string]any{
		"q": "sym Run | read", "contract_version": "q-v1", "workspace": "ws",
		"budget": 1000, "session_id": "s1",
	})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	want := []string{"q", "sym Run | read", "--format", "json", "--client-name", "mi-lsp-mcp", "--budget", "1000", "--session-id", "s1", "--workspace", "ws"}
	if len(got) != len(want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv = %#v, want %#v", got, want)
		}
	}
}

func TestMCPMilspToolSchema(t *testing.T) {
	listed, err := mcp.ToolList()
	if err != nil {
		t.Fatalf("ToolList: %v", err)
	}
	for _, tool := range listed {
		if tool.Name != "milsp" {
			continue
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("milsp schema: %v", err)
		}
		if _, ok := schema.Properties["q"]; !ok {
			t.Fatal("milsp schema is missing q")
		}
		if len(schema.Required) != 1 || schema.Required[0] != "q" {
			t.Fatalf("milsp required = %#v, want [q]", schema.Required)
		}
		return
	}
	t.Fatal("milsp tool not found")
}
