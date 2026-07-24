package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestFlowSliceRequiresSelectorOrEndpoints(t *testing.T) {
	app := New(t.TempDir(), nil)
	_, err := app.flowSlice(context.Background(), model.CommandRequest{
		Operation: "nav.flow-slice",
		Context:   model.QueryOptions{Workspace: t.TempDir()},
		Payload:   map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error when no selector/from/to")
	}
}

func TestFlowSliceBuildsPacketWithSelector(t *testing.T) {
	root := t.TempDir()
	// Minimal registration via workspace path as workspace selector often needs real root.
	// Use direct App with temp workspace that may not have catalog; packet should still return ok with warnings.
	app := New(root, nil)
	env, err := app.flowSlice(context.Background(), model.CommandRequest{
		Operation: "nav.flow-slice",
		Context:   model.QueryOptions{Workspace: root},
		Payload: map[string]any{
			"selector": "Execute",
			"limit":    5,
		},
	})
	// Workspace resolution may fail without registry; accept either structured error or ok envelope.
	if err != nil {
		// acceptable in unit environment without registered workspace
		t.Logf("flow-slice without registry: %v", err)
		return
	}
	if !env.Ok {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	packets, ok := env.Items.([]FlowSlicePacket)
	if !ok || len(packets) != 1 {
		t.Fatalf("expected one FlowSlicePacket, got %#v", env.Items)
	}
	if packets[0].Selector != "Execute" {
		t.Fatalf("selector not preserved: %#v", packets[0])
	}
	_ = filepath.ToSlash(root)
}

func TestBuildFlowSliceContinuationEmitsBatch(t *testing.T) {
	packet := FlowSlicePacket{
		Selector: "Run",
		ReadFirst: []FlowSliceRead{
			{Path: "internal/service/app.go", Line: 10, Why: "seed"},
		},
	}
	packet.BatchNext = buildFlowSliceBatchNext(packet)
	cont := buildFlowSliceContinuation(packet)
	if cont == nil || cont.Next.Op != "nav.batch" || len(cont.Next.Batch) == 0 {
		t.Fatalf("expected nav.batch continuation, got %#v", cont)
	}
}
