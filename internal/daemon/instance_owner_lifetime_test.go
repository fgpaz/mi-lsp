package daemon

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInstanceLeaseDifferentEndpointsSameDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-lease-a-%d", os.Getpid()))
	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	first, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, options)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Shutdown()
	t.Setenv("USERNAME", fmt.Sprintf("aorph-lease-b-%d", os.Getpid()))
	started := time.Now()
	second, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, options)
	if second != nil {
		second.Shutdown()
		t.Fatal("second constructor opened the same daemon storage")
	}
	if err == nil || !strings.Contains(err.Error(), "instance owner lease") {
		t.Fatalf("expected occupied lifetime lease, got %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("occupied lease did not reject in bounded time")
	}
	first.Shutdown()
	first.Shutdown()
	path, err := daemonInstanceLockPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("own lease remained after shutdown: %v", err)
	}
	third, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, options)
	if err != nil {
		t.Fatalf("storage not reusable after shutdown: %v", err)
	}
	third.Shutdown()
}

func TestInstanceLeaseReleasedOnConstructorFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-lease-failed-%d", os.Getpid()))
	path, err := daemonStatePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	server, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, DefaultStartOptions())
	if server != nil {
		server.Shutdown()
		t.Fatal("malformed state accepted")
	}
	if err == nil {
		t.Fatal("missing malformed state error")
	}
	lease, err := daemonInstanceLockPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lease); !os.IsNotExist(err) {
		t.Fatalf("lease leaked on constructor failure: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	server, err = NewServerWithOptions(t.TempDir(), 1, time.Minute, DefaultStartOptions())
	if err != nil {
		t.Fatalf("own storage not reusable after construction failure: %v", err)
	}
	server.Shutdown()
}
