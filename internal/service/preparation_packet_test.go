package service

import (
	"context"
	"encoding/json"
	"github.com/fgpaz/mi-lsp/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparationCreateVerifyRoundTripAndCanonicalReceipt(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, filepath.Join(".docs", "wiki", "00_gobierno_documental.md"), "governance")
	out := filepath.Join(root, "packet.json")
	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{Operation: "prepare.create", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"task": "query", "intent": "read_only", "output": out, "affected_paths": []string{"src/Hello.cs"}}})
	if err != nil || !env.Ok {
		t.Fatalf("create: %#v %v", env, err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
	env, err = app.Execute(context.Background(), model.CommandRequest{Operation: "prepare.verify", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"packet_path": out, "task": "query"}})
	if err != nil || !env.Ok {
		t.Fatalf("verify: %#v %v", env, err)
	}
	var p model.PreparationPacket
	b, _ := os.ReadFile(out)
	if json.Unmarshal(b, &p) != nil || p.PacketDigest == "" {
		t.Fatalf("bad packet")
	}
}
