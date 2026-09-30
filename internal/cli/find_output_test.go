package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/spf13/cobra"
)

func TestNavFindJSONKeepsNoMatchDistinctFromIndexErrors(t *testing.T) {
	indexHint := "Index the registered workspace with `mi-lsp index --workspace <workspace>` and retry `mi-lsp nav find <pattern> --workspace <workspace>`."
	tests := []struct {
		name     string
		result   model.Envelope
		wantErr  bool
		wantHit  bool
		wantCode string
	}{
		{
			name: "indexed workspace with no matches",
			result: model.Envelope{
				Ok:        true,
				Workspace: "demo",
				Backend:   "catalog",
				Items:     []model.SymbolRecord{},
			},
		},
		{
			name: "indexed workspace with a match",
			result: model.Envelope{
				Ok:        true,
				Workspace: "demo",
				Backend:   "catalog",
				Items: []model.SymbolRecord{{
					Name:      "NeedleSymbol",
					Kind:      "function",
					FilePath:  "sample.go",
					StartLine: 3,
				}},
			},
			wantHit: true,
		},
		{
			name: "registered workspace without catalog generation",
			result: model.Envelope{
				Ok:        false,
				Workspace: "demo",
				Backend:   "catalog",
				Items:     []model.SymbolRecord{},
				Error: &model.EnvelopeError{
					Kind:       "index",
					Code:       "index_not_ready",
					Message:    "index_not_ready",
					Stage:      "catalog",
					HintCode:   "index_not_ready",
					ReasonCode: "explicit_incomplete",
					Detail:     indexHint,
				},
				NextHint: &indexHint,
			},
			wantErr:  true,
			wantCode: "index_not_ready",
		},
		{
			name: "catalog generation state unavailable",
			result: model.Envelope{
				Ok:        false,
				Workspace: "demo",
				Backend:   "catalog",
				Items:     []model.SymbolRecord{},
				Error: &model.EnvelopeError{
					Kind:       "index",
					Code:       "workspace_db_open_failed",
					Message:    "workspace_db_open_failed",
					Stage:      "catalog",
					HintCode:   "workspace_db_open_failed",
					ReasonCode: "explicit_incomplete",
					Detail:     indexHint,
				},
				NextHint: &indexHint,
			},
			wantErr:  true,
			wantCode: "workspace_db_open_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, restore := captureFindStdout(t)
			state := &rootState{
				format:         "json",
				formatExplicit: true,
				clientName:     "test",
				noDaemon:       true,
				telemetry:      &CLITelemetry{},
				retentionRun:   true,
				appExecute: func(context.Context, model.CommandRequest) (model.Envelope, error) {
					return tc.result, nil
				},
			}
			cmd := &cobra.Command{}
			cmd.Flags().StringVar(&state.format, "format", "compact", "")
			if err := cmd.Flags().Set("format", "json"); err != nil {
				t.Fatalf("set format: %v", err)
			}
			cmd.SetContext(context.Background())
			execErr := state.executeOperation(cmd, "nav.find", map[string]any{"pattern": "Needle"}, true)
			encoded, err := stdout()
			restore()
			if err != nil {
				t.Fatalf("read command output: %v", err)
			}
			if tc.wantErr != IsEnvelopePrintedError(execErr) {
				t.Fatalf("executeOperation error = %v, want printed-error=%v", execErr, tc.wantErr)
			}

			var got model.Envelope
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatalf("decode JSON envelope: %v; output=%s", err, encoded)
			}
			if got.Ok == tc.wantErr {
				t.Fatalf("ok=%v, want %v", got.Ok, !tc.wantErr)
			}
			if got.Backend != "catalog" || got.Workspace != "demo" {
				t.Fatalf("backend/workspace = %q/%q, want catalog/demo", got.Backend, got.Workspace)
			}
			items, ok := got.Items.([]any)
			wantItemCount := 0
			if tc.wantHit {
				wantItemCount = 1
			}
			if !ok || len(items) != wantItemCount {
				t.Fatalf("items = %#v, want %d JSON item(s)", got.Items, wantItemCount)
			}
			if tc.wantHit {
				item, ok := items[0].(map[string]any)
				if !ok || item["name"] != "NeedleSymbol" || item["file_path"] != "sample.go" || item["line"] != float64(3) {
					t.Fatalf("match item = %#v, want NeedleSymbol at sample.go:3", items[0])
				}
			}
			if tc.wantErr {
				if got.Error == nil || got.Error.Code != tc.wantCode {
					t.Fatalf("error = %#v, want typed index error %q", got.Error, tc.wantCode)
				}
				if got.Error.ReasonCode != "explicit_incomplete" || got.Error.Detail != indexHint {
					t.Fatalf("error reason/detail = %q/%q, want explicit_incomplete/actionable hint", got.Error.ReasonCode, got.Error.Detail)
				}
				if got.NextHint == nil || *got.NextHint != indexHint {
					t.Fatalf("next_hint = %#v, want actionable index command", got.NextHint)
				}
			} else if got.Error != nil {
				t.Fatalf("zero-match result unexpectedly has error: %+v", *got.Error)
			}
		})
	}
}

func captureFindStdout(t *testing.T) (func() ([]byte, error), func()) {
	t.Helper()
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writePipe
	return func() ([]byte, error) {
		if err := writePipe.Close(); err != nil {
			return nil, err
		}
		return io.ReadAll(readPipe)
	}, func() {
		os.Stdout = original
		_ = writePipe.Close()
		_ = readPipe.Close()
	}
}
