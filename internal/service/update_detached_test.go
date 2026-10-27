package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
)

const detachedProofParent = "A15_DETACHED_PROOF_PARENT"
const detachedProofChild = "A15_DETACHED_PROOF_CHILD"
const detachedProofRoot = "A15_DETACHED_PROOF_ROOT"

type detachedProofMetadata struct {
	PID       int      `json:"pid"`
	Address   string   `json:"address,omitempty"`
	CWD       string   `json:"cwd,omitempty"`
	Args      []string `json:"args,omitempty"`
	Client    string   `json:"client,omitempty"`
	StdinNull bool     `json:"stdin_null,omitempty"`
	StdoutLog bool     `json:"stdout_log,omitempty"`
	StderrLog bool     `json:"stderr_log,omitempty"`
}

func detachedProofWrite(path string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Only the helper modes bypass testing's flag parsing; ordinary service tests
// execute normally. The child is the exact test executable launched by production.
func TestMain(m *testing.M) {
	if os.Getenv(detachedProofChild) == "1" {
		os.Exit(detachedProofChildMain())
	}
	if os.Getenv(detachedProofParent) == "1" {
		os.Exit(detachedProofParentMain())
	}
	os.Exit(m.Run())
}

func detachedProofParentMain() int {
	root := os.Getenv(detachedProofRoot)
	pid, err := startDetachedIndexJobProcessWithEnv(model.WorkspaceRegistration{Root: root, Name: "fixture"}, "proof-job", "fixture", []string{detachedProofChild + "=1"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err = detachedProofWrite(filepath.Join(root, "parent.json"), detachedProofMetadata{PID: pid, CWD: detachedIndexJobCWD()}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func detachedProofChildMain() int {
	root := os.Getenv(detachedProofRoot)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 2
	}
	defer listener.Close()
	tcp, ok := listener.(*net.TCPListener)
	if !ok {
		return 3
	}
	if err = tcp.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return 4
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 5
	}
	logInfo, err := os.Stat(filepath.Join(root, ".mi-lsp", "index-jobs", "proof-job.log"))
	if err != nil {
		return 6
	}
	inInfo, err := os.Stdin.Stat()
	if err != nil {
		return 7
	}
	outInfo, err := os.Stdout.Stat()
	if err != nil {
		return 7
	}
	errInfo, err := os.Stderr.Stat()
	if err != nil {
		return 8
	}
	metadata := detachedProofMetadata{PID: os.Getpid(), Address: listener.Addr().String(), CWD: cwd, Args: os.Args[1:], Client: os.Getenv("MI_LSP_CLIENT_NAME"), StdinNull: inInfo.Mode()&os.ModeCharDevice != 0, StdoutLog: os.SameFile(logInfo, outInfo), StderrLog: os.SameFile(logInfo, errInfo)}
	fmt.Fprintln(os.Stdout, "proof-stdout")
	fmt.Fprintln(os.Stderr, "proof-stderr")
	if err = detachedProofWrite(filepath.Join(root, "ready.json"), metadata); err != nil {
		return 9
	}
	conn, err := listener.Accept()
	if err != nil {
		return 10
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return 11
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || line != "ping\n" {
		return 12
	}
	if _, err = fmt.Fprintln(conn, "alive"); err != nil {
		return 13
	}
	if err = conn.Close(); err != nil {
		return 14
	}
	if err = detachedProofWrite(filepath.Join(root, "done.json"), detachedProofMetadata{PID: os.Getpid()}); err != nil {
		return 15
	}
	return 0
}

func detachedProofAwait(t *testing.T, path string) detachedProofMetadata {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		b, err := os.ReadFile(path)
		if err == nil {
			var result detachedProofMetadata
			if e := json.Unmarshal(b, &result); e != nil {
				t.Fatal(e)
			}
			return result
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		select {
		case <-timer.C:
			t.Fatalf("timeout waiting for fixture %s", filepath.Base(path))
		case <-ticker.C:
		}
	}
}

func TestDetachedIndexJobSurvivesParentExit(t *testing.T) {
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := processutil.Command(exe)
	for _, e := range os.Environ() {
		upper := strings.ToUpper(e)
		if strings.HasPrefix(upper, detachedProofParent+"=") || strings.HasPrefix(upper, detachedProofChild+"=") || strings.HasPrefix(upper, detachedProofRoot+"=") {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	cmd.Env = append(cmd.Env, detachedProofParent+"=1", detachedProofRoot+"="+root)
	cmd.Dir = root
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	parentPID := cmd.Process.Pid
	parentDone := make(chan error, 1)
	go func() { parentDone <- cmd.Wait() }()
	select {
	case err = <-parentDone:
		if err != nil {
			t.Fatalf("parent helper failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent helper exit timeout")
	}
	parent := detachedProofAwait(t, filepath.Join(root, "parent.json"))
	child := detachedProofAwait(t, filepath.Join(root, "ready.json"))
	if child.PID <= 0 || child.PID != parent.PID || child.PID == parentPID || child.PID == os.Getpid() {
		t.Fatalf("unexpected helper identities parent=%d launched=%d child=%d", parentPID, parent.PID, child.PID)
	}
	if !strings.EqualFold(filepath.Clean(child.CWD), filepath.Clean(parent.CWD)) || strings.EqualFold(filepath.Clean(child.CWD), filepath.Clean(root)) {
		t.Fatalf("child cwd not neutral: %q", child.CWD)
	}
	if strings.Join(child.Args, " ") != "--workspace fixture --format json index run-job proof-job" || child.Client != "mi-lsp-index-job" || !child.StdinNull || !child.StdoutLog || !child.StderrLog {
		t.Fatalf("detached command mismatch: %+v", child)
	}
	conn, err := net.DialTimeout("tcp", child.Address, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err = fmt.Fprintln(conn, "ping"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || line != "alive\n" {
		t.Fatalf("child did not respond after parent exit: %q %v", line, err)
	}
	_ = conn.Close()
	done := detachedProofAwait(t, filepath.Join(root, "done.json"))
	if done.PID != child.PID {
		t.Fatalf("unexpected done pid %d", done.PID)
	}
	log, err := os.ReadFile(filepath.Join(root, ".mi-lsp", "index-jobs", "proof-job.log"))
	if err != nil || !strings.Contains(string(log), "proof-stdout") || !strings.Contains(string(log), "proof-stderr") {
		t.Fatalf("detached output not in fixture log: %v", err)
	}
}
