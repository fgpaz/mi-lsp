package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestDiagnosticsCommandPayload(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr bool
		changed bool
		file    string
	}{
		{name: "file", args: []string{"src/file.go"}, file: "src/file.go"},
		{name: "changed", args: []string{"--changed", "--base", "origin/main", "--max-files", "8", "--offset", "2"}, changed: true},
		{name: "empty", wantErr: true},
		{name: "both", args: []string{"a.go", "--changed"}, wantErr: true},
		{name: "base_without_changed", args: []string{"a.go", "--base", "HEAD"}, wantErr: true},
		{name: "unbounded", args: []string{"--changed", "--max-files", "1000"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			state := &rootState{executeOperationHook: func(_ *cobra.Command, operation string, payload map[string]any, daemon bool) error {
				called = true
				if operation != "nav.diagnostics" || !daemon || payload["changed"] != tc.changed {
					t.Fatalf("operation=%s payload=%v", operation, payload)
				}
				if tc.file != "" && payload["file"] != tc.file {
					t.Fatal(payload)
				}
				return nil
			}}
			cmd := newDiagnosticsCommand(state)
			cmd.SetArgs(tc.args)
			e := cmd.Execute()
			if (e != nil) != tc.wantErr || called == tc.wantErr {
				t.Fatalf("called=%v err=%v", called, e)
			}
		})
	}
}
