package daemon

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

func TestConstructorErrorRetainsBothLeasesUntilDatabaseDriverJoins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERNAME", "aorph-constructor-lifetime")

	gate := &drainConnector{
		path: filepath.Join(home, "constructor.db"), entered: make(chan struct{}),
		release: make(chan struct{}), canceled: make(chan struct{}),
	}
	store := &TelemetryStore{}
	openTelemetryStoreForServer = func() (*TelemetryStore, error) {
		store.db = sql.OpenDB(gate)
		store.db.SetMaxOpenConns(1)
		store.db.SetMaxIdleConns(1)
		store.writeQueue = make(chan telemetryWriteRequest, 8)
		store.done = make(chan struct{})
		store.walMaxBytes = telemetryWALMaxBytes
		if err := store.configureConnection(); err != nil {
			return nil, err
		}
		if err := store.initSchema(); err != nil {
			return nil, err
		}
		store.wg.Add(1)
		go store.asyncWriteWorker()
		return store, nil
	}
	defer func() { openTelemetryStoreForServer = openTelemetryStore }()

	enteredToken := make(chan struct{})
	failToken := make(chan struct{})
	constructorErr := errors.New("injected admin token construction error")
	generateServerAdminToken = func() (string, error) {
		close(enteredToken)
		<-failToken
		return "", constructorErr
	}
	defer func() { generateServerAdminToken = generateAdminToken }()

	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	result := make(chan error, 1)
	go func() {
		_, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, options)
		result <- err
	}()
	defer func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
	}()

	select {
	case <-enteredToken:
	case <-time.After(3 * time.Second):
		t.Fatal("constructor did not reach the post-database constructor failure seam")
	}
	if err := store.RecordAccess(0, model.AccessEvent{SessionID: "ctor", Operation: "held", OccurredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("database write did not enter its controlled driver call")
	}
	close(failToken)
	select {
	case <-gate.canceled:
	case <-time.After(telemetryCloseDrainTimeout + time.Second):
		t.Fatal("constructor cleanup did not cancel the bounded database operation")
	}
	select {
	case err := <-result:
		t.Fatalf("constructor returned before actual database driver join: %v", err)
	default:
	}

	lifePath, err := daemonInstanceLifeGuardPath()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	if other, err := runtimeinfo.AcquireFileLeaseContext(ctx, lifePath); err == nil {
		_ = other.Release()
		cancel()
		t.Fatal("OS lease was available before constructor's database join")
	}
	cancel()
	cooperativePath, err := daemonInstanceLockPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireStartLockAt(cooperativePath, 100*time.Millisecond, time.Now, processExists); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("cooperative instance lease was not retained during constructor cleanup: %v", err)
	}

	close(gate.release)
	select {
	case err := <-result:
		if !errors.Is(err, constructorErr) {
			t.Fatalf("constructor failure not preserved: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("constructor cleanup did not finish after driver release")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	other, err := runtimeinfo.AcquireFileLeaseContext(ctx, lifePath)
	if err != nil {
		t.Fatalf("OS lifetime lease remained after actual database join: %v", err)
	}
	_ = other.Release()
	if _, err := os.Stat(cooperativePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cooperative lease remained after constructor cleanup: %v", err)
	}
}
