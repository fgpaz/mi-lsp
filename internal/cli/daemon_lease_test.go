package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func leaseQueryPrivateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-lease-query-%d", os.Getpid()))
	return home
}

// Exercise only existing Cobra construction/execution APIs. Until the lease
// command exists this fails behaviorally, not because a future Go API is absent.
func executeLeaseQuery(t *testing.T) (map[string]any, []byte) {
	return executeLeaseQueryWithBudget(t, 1<<20)
}

func executeLeaseQueryWithBudget(t *testing.T, maxChars int) (map[string]any, []byte) {
	t.Helper()
	capture, err := os.CreateTemp(t.TempDir(), "lease-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	oldStdout := os.Stdout
	os.Stdout = capture
	defer func() { os.Stdout = oldStdout }()
	state := &rootState{repoRoot: t.TempDir(), format: "json", formatExplicit: true, maxChars: maxChars}
	command := newDaemonCommand(state)
	// Match real root flag plumbing: effectiveFormat checks Changed("format"),
	// not the cached formatExplicit field, before implicit agent/AXI defaults.
	command.PersistentFlags().StringVar(&state.format, "format", "json", "Output format")
	command.SetArgs([]string{"lease", "--format", "json"})
	command.SetOut(capture)
	command.SetErr(io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command.SetContext(ctx)
	if err := command.Execute(); err != nil {
		t.Fatalf("readonly lease query must succeed without daemon IPC: %v", err)
	}
	os.Stdout = oldStdout
	if _, err := capture.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(capture)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK    bool             `json:"ok"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("lease query did not return intact management JSON: %v", err)
	}
	if !envelope.OK || len(envelope.Items) != 1 {
		t.Fatalf("expected one successful readonly lease result, got ok=%v items=%d", envelope.OK, len(envelope.Items))
	}
	item := envelope.Items[0]
	if item["schema"] != "mi-lsp-daemon-lease/v1" {
		t.Fatalf("missing stable client lease schema: %v", item["schema"])
	}
	allowed := map[string]bool{"schema": true, "state": true, "owner_pid": true, "instance_id": true, "liveness": true, "endpoint": true}
	for key := range item {
		if !allowed[key] {
			t.Fatalf("lease output contains non-contract field %q", key)
		}
	}
	return item, body
}

func TestDaemonLeaseQueryAbsentDoesNotCreateRuntime(t *testing.T) {
	home := leaseQueryPrivateHome(t)
	item, _ := executeLeaseQuery(t)
	if item["state"] != "absent" {
		t.Fatalf("expected absent lease, got %v", item["state"])
	}
	if _, err := os.Stat(filepath.Join(home, ".mi-lsp")); !os.IsNotExist(err) {
		t.Fatalf("readonly absent query created runtime state or failed stat: %v", err)
	}
}

func TestDaemonLeaseQueryPublishedOwnerWithoutIPC(t *testing.T) {
	home := leaseQueryPrivateHome(t)
	dir := filepath.Join(home, ".mi-lsp", "daemon")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Real metadata format emitted by writeStartLockMetadata, not invented JSON.
	leaseBody := []byte(fmt.Sprintf("version=1\npid=%d\nnonce=00112233445566778899aabbccddeeff\n", os.Getpid()))
	stateBody, err := json.Marshal(map[string]any{
		"pid": os.Getpid(), "endpoint": "aorph-lease-no-listener", "version": "0.16.2",
		"admin_token": "OWN-FIXTURE-ADMIN-TOKEN-MUST-NOT-BE-RENDERED",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"instance.lock": leaseBody, "state.json": stateBody} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	item, body := executeLeaseQuery(t)
	if item["state"] != "active" || item["owner_pid"] != float64(os.Getpid()) {
		t.Fatalf("published lease identity was not reported: state=%v pid=%v", item["state"], item["owner_pid"])
	}
	if item["liveness"] != "alive" && item["liveness"] != "indeterminate" {
		t.Fatalf("live fixture owner was treated as absent/dead: %v", item["liveness"])
	}
	identity, ok := item["instance_id"].(string)
	if !ok || identity == "" || item["endpoint"] != "aorph-lease-no-listener" {
		t.Fatal("query omitted stable lease identity or safe endpoint")
	}
	if bytes.Contains(body, []byte("OWN-FIXTURE-ADMIN-TOKEN")) || bytes.Contains(body, []byte("admin_token")) {
		t.Fatal("lease result exposed an authentication field")
	}
	second, _ := executeLeaseQuery(t)
	if second["instance_id"] != identity {
		t.Fatal("readonly queries changed the reported lease identity")
	}
	for name, expected := range map[string][]byte{"instance.lock": leaseBody, "state.json": stateBody} {
		actual, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("readonly query changed %s: %v", name, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("lease query created DB/guard/runtime artifacts: entries=%d err=%v", len(entries), err)
	}
}

func TestDaemonLeaseQueryDoesNotStartAutomaticUpdate(t *testing.T) {
	if shouldStartAutomaticUpdate([]string{"lease", "daemon"}, "0.16.2", false) {
		t.Fatal("readonly lease query must not launch automatic updater")
	}
}

func TestDaemonLeaseQueryPreservesSmallJSONBudget(t *testing.T) {
	leaseQueryPrivateHome(t)
	item, body := executeLeaseQueryWithBudget(t, 1)
	if item["schema"] != "mi-lsp-daemon-lease/v1" || !json.Valid(body) {
		t.Fatalf("lease JSON was cut by output budget: %q", body)
	}
}

func TestRootDaemonLeasePreRunIsReadonly(t *testing.T) {
	home := leaseQueryPrivateHome(t)
	root := NewRootCommand()
	started := false
	setUpdateAutomaticStartForTest(root, "0.16.2", func() error { started = true; return nil })
	capture, err := os.CreateTemp(t.TempDir(), "root-lease-output-")
	if err != nil { t.Fatal(err) }
	defer capture.Close()
	oldStdout := os.Stdout
	os.Stdout = capture
	defer func() { os.Stdout = oldStdout }()
	root.SetArgs([]string{"daemon", "lease", "--format", "json", "--max-chars", "1"})
	root.SetOut(capture)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil { t.Fatalf("root lease command failed: %v", err) }
	os.Stdout = oldStdout
	if started { t.Fatal("root pre-run started the automatic updater for lease query") }
	if _, err := os.Stat(filepath.Join(home, ".mi-lsp")); !os.IsNotExist(err) { t.Fatalf("root lease pre-run created runtime state: %v", err) }
	if _, err := capture.Seek(0, 0); err != nil { t.Fatal(err) }
	body, err := io.ReadAll(capture)
	if err != nil { t.Fatal(err) }
	if !json.Valid(body) { t.Fatalf("root lease JSON was truncated by tiny output budget: %q", body) }
}
