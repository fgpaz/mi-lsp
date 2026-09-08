package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestIndexEntrypointFlagForwardsSelectorForWrapperAndStart(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		wait bool
	}{
		{name: "wrapper", args: []string{"--entrypoint", "go.mod"}, wait: true},
		{name: "start", args: []string{"start", "--entrypoint", "repo::go", "--mode", "full"}, wait: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var operation string
			var payload map[string]any
			state := &rootState{repoRoot: t.TempDir(), workspace: t.TempDir(), executeOperationHook: func(_ *cobra.Command, gotOperation string, gotPayload map[string]any, _ bool) error {
				operation, payload = gotOperation, gotPayload
				return nil
			}}
			command := newIndexCommand(state)
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			want := "go.mod"
			if test.name == "start" {
				want = "repo::go"
			}
			if operation != "index.start" || payload["entrypoint"] != want {
				t.Fatalf("operation=%q payload=%#v, want index.start entrypoint=%q", operation, payload, want)
			}
			if payload["wait"] != test.wait {
				t.Fatalf("payload wait=%#v, want %v", payload["wait"], test.wait)
			}
		})
	}
}
