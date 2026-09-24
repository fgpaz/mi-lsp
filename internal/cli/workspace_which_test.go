package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

const windowsPluginsPath = `C:\repos\mios\mis-plugins-cc`

func TestWorkspaceWhichCommandDoesNotUseDaemon(t *testing.T) {
	state := &rootState{executeOperationHook: func(_ *cobra.Command, operation string, _ map[string]any, _ bool) error {
		t.Fatalf("workspace which called %s", operation)
		return nil
	}}
	command := newWorkspaceCommand(state)
	which, _, err := command.Find([]string{"which"})
	if err != nil {
		t.Fatalf("find workspace which: %v", err)
	}
	if which.Use != "which" {
		t.Fatalf("Use=%q, want which", which.Use)
	}
	if which.Flags().Lookup("cwd") == nil {
		t.Fatal("workspace which should expose --cwd")
	}
}

func TestWorkspaceWhichPreservesWindowsBackslashPath(t *testing.T) {
	isolateWorkspaceHome(t)
	registerWindowsPluginsWorkspace(t)

	state := &rootState{
		workspace: "mis-plugins",
		format:    "json",
		executeOperationHook: func(_ *cobra.Command, operation string, _ map[string]any, _ bool) error {
			t.Fatalf("workspace which called %s", operation)
			return nil
		},
	}
	command := newWorkspaceCommand(state)
	which, _, err := command.Find([]string{"which"})
	if err != nil {
		t.Fatalf("find workspace which: %v", err)
	}

	rendered := captureStdout(t, func() error {
		return which.RunE(which, nil)
	})
	if strings.Contains(rendered, `C:\reposmiosmis-plugins-cc`) {
		t.Fatalf("workspace root collapsed JSON escapes: %s", rendered)
	}
	if !strings.Contains(rendered, `C:\\repos\\mios\\mis-plugins-cc`) {
		t.Fatalf("JSON output = %s, want escaped Windows path", rendered)
	}

	var envelope struct {
		Items []struct {
			Name       string `json:"name"`
			Root       string `json:"root"`
			Executable string `json:"executable"`
			Source     string `json:"source"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(rendered), &envelope); err != nil {
		t.Fatalf("unmarshal which output: %v\n%s", err, rendered)
	}
	if len(envelope.Items) != 1 {
		t.Fatalf("items = %#v", envelope.Items)
	}
	item := envelope.Items[0]
	if item.Name != "mis-plugins" {
		t.Fatalf("name = %q", item.Name)
	}
	if item.Root != windowsPluginsPath {
		t.Fatalf("root = %q, want %q", item.Root, windowsPluginsPath)
	}
	for _, segment := range []string{`\repos`, `\mios`, `\mis-plugins`} {
		if !strings.Contains(item.Root, segment) {
			t.Fatalf("root %q lost segment %s", item.Root, segment)
		}
	}
	if item.Source != string(workspace.ResolutionSourceExplicit) {
		t.Fatalf("source = %q", item.Source)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	if item.Executable != filepath.Clean(exe) {
		t.Fatalf("executable = %q, want %q", item.Executable, filepath.Clean(exe))
	}
	if strings.Contains(fmtQuoted(item.Root), `C:\reposmiosmis-plugins-cc`) {
		t.Fatalf("quoted root collapsed: %s", fmtQuoted(item.Root))
	}
}

func TestWorkspaceWhichResolvesCWDAndPathSelector(t *testing.T) {
	isolateWorkspaceHome(t)
	registerWindowsPluginsWorkspace(t)

	byCWD, err := resolveWorkspaceWhich("", windowsPluginsPath+`\plugins\claude-code-milsp`)
	if err != nil {
		t.Fatalf("resolve cwd: %v", err)
	}
	if byCWD.Name != "mis-plugins" || byCWD.Root != windowsPluginsPath {
		t.Fatalf("cwd resolution = %#v", byCWD)
	}
	if byCWD.Source != string(workspace.ResolutionSourceCallerCWD) {
		t.Fatalf("source = %q", byCWD.Source)
	}

	byPath, err := resolveWorkspaceWhich(windowsPluginsPath, "")
	if err != nil {
		t.Fatalf("resolve path selector: %v", err)
	}
	if byPath.Root != windowsPluginsPath || byPath.Name != "mis-plugins" {
		t.Fatalf("path resolution = %#v", byPath)
	}

	byLast, err := resolveWorkspaceWhich("", `D:\unrelated`)
	if err != nil {
		t.Fatalf("resolve last workspace: %v", err)
	}
	if byLast.Name != "mis-plugins" || byLast.Root != windowsPluginsPath {
		t.Fatalf("last workspace = %#v", byLast)
	}
	if byLast.Source != string(workspace.ResolutionSourceLastWorkspace) {
		t.Fatalf("source = %q", byLast.Source)
	}
}

func TestLiteralWorkspacePathDoesNotUnquoteJSONEscapes(t *testing.T) {
	got := literalWorkspacePath(windowsPluginsPath)
	if got != windowsPluginsPath {
		t.Fatalf("literal = %q", got)
	}
	collapsed := lenientJSONUnescape(windowsPluginsPath)
	if collapsed == windowsPluginsPath || !strings.Contains(fmtQuoted(collapsed), `C:\reposmiosmis-plugins-cc`) {
		t.Fatalf("regression setup collapsed = %q (%s)", collapsed, fmtQuoted(collapsed))
	}
	if got == collapsed {
		t.Fatal("literal workspace path collapsed \\r \\m \\m escapes")
	}
}

func isolateWorkspaceHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func registerWindowsPluginsWorkspace(t *testing.T) {
	t.Helper()
	_, err := workspace.RegisterWorkspace("mis-plugins", model.WorkspaceRegistration{Root: windowsPluginsPath})
	if err != nil {
		t.Fatalf("register workspace: %v", err)
	}
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writePipe
	copyDone := make(chan struct{})
	var body []byte
	go func() {
		body, _ = io.ReadAll(readPipe)
		close(copyDone)
	}()
	fnErr := fn()
	_ = writePipe.Close()
	os.Stdout = original
	<-copyDone
	_ = readPipe.Close()
	if fnErr != nil {
		t.Fatalf("command: %v", fnErr)
	}
	return string(body)
}

func fmtQuoted(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return string(encoded)
}

func lenientJSONUnescape(path string) string {
	var builder strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '\\' || i+1 >= len(path) {
			builder.WriteByte(path[i])
			continue
		}
		next := path[i+1]
		i++
		switch next {
		case 'r':
			builder.WriteByte('\r')
		case 'n':
			builder.WriteByte('\n')
		case 't':
			builder.WriteByte('\t')
		case '\\', '"', '/':
			builder.WriteByte(next)
		default:
			builder.WriteByte(next)
		}
	}
	return builder.String()
}
