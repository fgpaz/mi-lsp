package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/selfupdate"
	"github.com/spf13/cobra"
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
			code := "UPDATE_NETWORK_UNAVAILABLE"
			if result == "deferred" {
				code = "UPDATE_IN_USE_DEFERRED"
			}
			now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
			calls := 0
			root := NewRootCommand()
			setUpdateCheckForTest(root, func(context.Context) (selfupdate.Status, error) {
				calls++
				return selfupdate.Status{Schema: "update-status/v1", Tool: "mi-lsp", Installed: "0.16.0", Latest: "0.16.1", LastCheck: &now, LastResult: result, Code: &code, Channel: "latest", Auto: true}, nil
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

// Exercise the actual root pre-run. The injected start only queues a child;
// a blocked child/network simulation must not delay the command itself.
func TestRootAutomaticUpdateHookDoesNotWaitForChild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := NewRootCommand()
	root.AddCommand(&cobra.Command{Use: "fixture-normal", RunE: func(*cobra.Command, []string) error { return nil }})
	childRelease := make(chan struct{})
	defer close(childRelease)
	started := make(chan struct{}, 1)
	setUpdateAutomaticStartForTest(root, "0.16.0", func() error {
		started <- struct{}{}
		go func() { <-childRelease }()
		return nil
	})
	root.SetArgs([]string{"fixture-normal"})
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("root waited for automatic child")
	}
	select {
	case <-started:
	default:
		t.Fatal("normal command did not invoke automatic hook")
	}
}

func TestRootAutomaticUpdateHookExclusions(t *testing.T) {
	for _, args := range [][]string{{"update", "--check"}, {"update"}, {"doctor"}, {"probe"}, {"repair"}, {"help"}} {
		t.Run(args[0], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			root := NewRootCommand()
			calls := 0
			setUpdateAutomaticStartForTest(root, "0.16.0", func() error { calls++; return nil })
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			cmd, _, err := root.Find(args)
			if err == nil && cmd != root {
				cmd.Run = nil
				cmd.RunE = func(*cobra.Command, []string) error { return nil }
			}
			root.SetArgs(args)
			_ = root.Execute()
			if calls != 0 {
				t.Fatalf("excluded command %v started automatic update", args)
			}
		})
	}
}
