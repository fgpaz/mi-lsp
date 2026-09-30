package cli

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSuggestNav(t *testing.T) {
	windowsPath := `C:\repos\mi-lsp\internal\cli\suggest.go`
	marshaled, err := json.Marshal(map[string]any{
		"file_path": windowsPath,
		"offset":    4,
		"limit":     2,
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	tests := []struct {
		name    string
		tool    string
		args    string
		want    []navSuggestItem
		wantErr bool
	}{
		{
			name: "read file_path",
			tool: "Read",
			args: `{"file_path":"internal/cli/nav.go"}`,
			want: []navSuggestItem{{
				Tool:    "Read",
				Command: "nav multi-read",
				Argv:    []string{"nav", "multi-read", "internal/cli/nav.go"},
				Reason:  suggestReasonRead,
			}},
		},
		{
			name: "read path with offset and limit",
			tool: "Read",
			args: `{"path":"internal/cli/nav.go","offset":10,"limit":5}`,
			want: []navSuggestItem{{
				Tool:    "Read",
				Command: "nav multi-read",
				Argv:    []string{"nav", "multi-read", "internal/cli/nav.go:10-14"},
				Reason:  suggestReasonRead,
			}},
		},
		{
			name: "read file",
			tool: "read",
			args: `{"file":"internal/cli/suggest.go"}`,
			want: []navSuggestItem{{
				Tool:    "Read",
				Command: "nav multi-read",
				Argv:    []string{"nav", "multi-read", "internal/cli/suggest.go"},
				Reason:  suggestReasonRead,
			}},
		},
		{
			name: "read windows backslash-r raw json",
			tool: "Read",
			args: `{"file_path":"C:\repos\mi-lsp\internal\cli\suggest.go"}`,
			want: []navSuggestItem{{
				Tool:    "Read",
				Command: "nav multi-read",
				Argv:    []string{"nav", "multi-read", windowsPath},
				Reason:  suggestReasonRead,
			}},
		},
		{
			name: "read windows backslash-r marshaled json",
			tool: "Read",
			args: string(marshaled),
			want: []navSuggestItem{{
				Tool:    "Read",
				Command: "nav multi-read",
				Argv:    []string{"nav", "multi-read", windowsPath + ":4-5"},
				Reason:  suggestReasonRead,
			}},
		},
		{
			name: "grep query",
			tool: "Grep",
			args: `{"query":"nav suggest"}`,
			want: []navSuggestItem{{
				Tool:    "Grep",
				Command: "nav search",
				Argv:    []string{"nav", "search", "nav suggest"},
				Reason:  suggestReasonGrep,
			}},
		},
		{
			name: "grep regex",
			tool: "Grep",
			args: `{"pattern":"foo.*bar","regex":true}`,
			want: []navSuggestItem{{
				Tool:    "Grep",
				Command: "nav search",
				Argv:    []string{"nav", "search", "--regex", "foo.*bar"},
				Reason:  suggestReasonGrepRegex,
			}},
		},
		{
			name: "glob symbol",
			tool: "Glob",
			args: `{"pattern":"suggestNav"}`,
			want: []navSuggestItem{{
				Tool:    "Glob",
				Command: "nav find",
				Argv:    []string{"nav", "find", "suggestNav"},
				Reason:  suggestReasonGlob,
			}},
		},
		{
			name: "glob path",
			tool: "Glob",
			args: `{"pattern":"**/*.go"}`,
		},
		{
			name: "unknown tool",
			tool: "Bash",
			args: `{"command":"rg foo"}`,
		},
		{
			name:    "invalid json",
			tool:    "Read",
			args:    `{"file_path":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := suggestNav(tt.tool, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("suggestNav error = nil, want invalid JSON")
				}
				return
			}
			if err != nil {
				t.Fatalf("suggestNav: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("items = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i].Tool != tt.want[i].Tool || got[i].Command != tt.want[i].Command || got[i].Reason != tt.want[i].Reason || !equalStringSlice(got[i].Argv, tt.want[i].Argv) {
					t.Fatalf("item = %#v, want %#v", got[i], tt.want[i])
				}
				for _, arg := range got[i].Argv {
					if strings.Contains(arg, "\r") {
						t.Fatalf("argv %q contains a carriage return", arg)
					}
				}
			}
		})
	}
}

func TestNavSuggestCommandEnvelope(t *testing.T) {
	command := newNavCommand(&rootState{})
	suggest, _, err := command.Find([]string{"suggest"})
	if err != nil {
		t.Fatalf("find suggest: %v", err)
	}
	if suggest.Flags().Lookup("tool") == nil || suggest.Flags().Lookup("args") == nil {
		t.Fatal("suggest should expose --tool and --args")
	}

	decoded, execErr := executeNavSuggest(t, "--tool", "Read", "--args", `{"file_path":"C:\repos\mi-lsp\internal\cli\suggest.go"}`)
	if execErr != nil {
		t.Fatalf("nav suggest Read: %v", execErr)
	}
	items := suggestEnvelopeItems(t, decoded)
	if len(items) != 1 {
		t.Fatalf("items = %#v, want one", items)
	}
	item := items[0].(map[string]any)
	if item["tool"] != "Read" || item["command"] != "nav multi-read" || item["reason"] != suggestReasonRead {
		t.Fatalf("item = %#v", item)
	}
	argv := stringArgv(t, item["argv"])
	wantPath := `C:\repos\mi-lsp\internal\cli\suggest.go`
	if !equalStringSlice(argv, []string{"nav", "multi-read", wantPath}) {
		t.Fatalf("argv = %#v, want nav multi-read %s", argv, wantPath)
	}
	if strings.Contains(argv[2], "\r") {
		t.Fatalf("path = %q, want backslash-r preserved", argv[2])
	}

	empty, execErr := executeNavSuggest(t, "--tool", "Write", "--args", `{}`)
	if execErr != nil {
		t.Fatalf("nav suggest unknown: %v", execErr)
	}
	if items := suggestEnvelopeItems(t, empty); len(items) != 0 {
		t.Fatalf("unknown tool items = %#v, want none", items)
	}

	globbed, execErr := executeNavSuggest(t, "--tool", "Glob", "--args", `{"pattern":"**/*.go"}`)
	if execErr != nil {
		t.Fatalf("nav suggest glob: %v", execErr)
	}
	if items := suggestEnvelopeItems(t, globbed); len(items) != 0 {
		t.Fatalf("path glob items = %#v, want none", items)
	}
}

func executeNavSuggest(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()

	command := NewRootCommand()
	command.SetArgs(append([]string{"nav", "suggest", "--format", "json", "--no-daemon"}, args...))
	execErr := command.Execute()
	_ = writer.Close()
	os.Stdout = original
	raw, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	if execErr != nil {
		return nil, execErr
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return decoded, nil
}

func suggestEnvelopeItems(t *testing.T, decoded map[string]any) []any {
	t.Helper()
	if decoded["ok"] != true {
		t.Fatalf("ok = %#v, want true", decoded["ok"])
	}
	items, ok := decoded["items"].([]any)
	if !ok {
		t.Fatalf("items = %#v, want array", decoded["items"])
	}
	return items
}

func stringArgv(t *testing.T, value any) []string {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("argv = %#v, want array", value)
	}
	argv := make([]string, len(raw))
	for i, arg := range raw {
		text, ok := arg.(string)
		if !ok {
			t.Fatalf("argv[%d] = %#v, want string", i, arg)
		}
		argv[i] = text
	}
	return argv
}

func equalStringSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
