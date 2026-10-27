package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/selfupdate"
)

// Private integration seams: updateCheck is injected into the root's update
// command and updateAutomaticStart into the root pre-run. Production must use
// the real updater when these test hooks are absent.
func TestUpdateCheckJSONReadOnly(t *testing.T) {
	for _, result := range []string{"failed", "deferred"} {
		t.Run(result, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			code := "NETWORK_UNAVAILABLE"
			if result == "deferred" {
				code = "IN_USE"
			}
			now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
			calls := 0
			root := NewRootCommand()
			setUpdateCheckForTest(root, func(context.Context) (selfupdate.Status, error) {
				calls++
				return selfupdate.Status{Schema: "update-status/v1", Tool: "mi-lsp", Installed: "0.16.0", Latest: "0.16.1", LastCheck: &now, LastResult: result, Code: &code, Channel: "stable", Auto: true}, nil
			})
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetArgs([]string{"update", "--check", "--format", "json"})
			if err := root.Execute(); err != nil {
				t.Fatalf("readable %s status: %v", result, err)
			}
			var got map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("JSON: %v", err)
			}
			want := []string{"schema", "tool", "installed", "latest", "last_check", "last_result", "code", "previous", "channel", "auto"}
			if len(got) != len(want) {
				t.Fatalf("fields: %v", got)
			}
			for _, key := range want {
				if _, ok := got[key]; !ok {
					t.Errorf("missing %s", key)
				}
			}
			if got["last_result"] != result || got["code"] != code {
				t.Fatalf("status: %v", got)
			}
			if calls != 1 {
				t.Fatalf("check calls = %d", calls)
			}
			entries, err := os.ReadDir(home)
			if err != nil || len(entries) != 0 {
				t.Fatalf("home changed: %v %v", entries, err)
			}
		})
	}
}

func TestAutomaticUpdateExcludedCommandsAndDevelopmentBuilds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		version string
		child   bool
		want    bool
	}{
		{"normal stable", []string{"version"}, "0.16.0", false, true},
		{"update check", []string{"update", "--check"}, "0.16.0", false, false},
		{"update", []string{"update"}, "0.16.0", false, false},
		{"doctor", []string{"doctor"}, "0.16.0", false, false},
		{"probe", []string{"probe"}, "0.16.0", false, false},
		{"repair", []string{"repair"}, "0.16.0", false, false},
		{"help", []string{"help"}, "0.16.0", false, false},
		{"readback", []string{"version"}, "0.16.0", true, false},
		{"dev", []string{"version"}, "dev", false, false},
		{"unknown", []string{"version"}, "unknown", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldStartAutomaticUpdate(tc.args, tc.version, tc.child); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAutomaticChildCommandIsDetachedAndSilent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cmd := newAutomaticUpdateChildCommand("fixture-executable")
	if cmd.Path != "fixture-executable" || !reflect.DeepEqual(cmd.Args, []string{"fixture-executable", "update"}) {
		t.Fatalf("child command: %q %q", cmd.Path, cmd.Args)
	}
	if cmd.Stdout != nil || cmd.Stderr != nil || cmd.Stdin != nil {
		t.Fatal("child inherited streams")
	}
	if !containsUpdateEnv(cmd, "MI_LSP_UPDATE_CHILD=1") {
		t.Fatal("missing child recursion marker")
	}
	if !isDetachedUpdateCommand(cmd) {
		t.Fatal("child must use detached/no-window process configuration")
	}
}

func containsUpdateEnv(cmd *exec.Cmd, entry string) bool {
	for _, v := range cmd.Env {
		if strings.EqualFold(v, entry) {
			return true
		}
	}
	return false
}
