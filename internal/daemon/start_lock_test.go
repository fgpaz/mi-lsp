package daemon

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAcquireStartLockWritesStrictVersionedMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.lock")
	lock, err := acquireStartLockAtWithNonce(path, time.Second, time.Now, func(int) bool { return true }, func() (string, error) {
		return "00112233445566778899aabbccddeeff", nil
	})
	if err != nil {
		t.Fatalf("acquireStartLockAtWithNonce: %v", err)
	}
	defer lock.Close()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	metadata, ok := parseStartLockMetadata(body)
	if !ok {
		t.Fatalf("metadata %q is not strict version 1 metadata", body)
	}
	if metadata.version != startLockMetadataVersion || metadata.pid != os.Getpid() || metadata.nonce != "00112233445566778899aabbccddeeff" {
		t.Fatalf("metadata = %+v", metadata)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "start.guard")); err != nil {
		t.Fatalf("start.guard was not persisted: %v", err)
	}
}

func TestStartLockLiveOwnerIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.lock")
	writeRawStartLock(t, path, "version=1\npid=4242\nnonce=00112233445566778899aabbccddeeff\n")

	_, err := acquireStartLockAt(path, 25*time.Millisecond, time.Now, func(pid int) bool { return pid == 4242 })
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("acquire with live owner error = %v, want timeout", err)
	}
	assertStartLockBody(t, path, "version=1\npid=4242\nnonce=00112233445566778899aabbccddeeff\n")
}

func TestReclaimStartLockRejectsPIDsOutsideSafeDomainWithoutLivenessCheck(t *testing.T) {
	tests := []struct {
		name    string
		pidText string
		known   bool
		wantPID int
	}{
		{name: "zero", pidText: "0"},
		{name: "negative", pidText: "-1"},
		{name: "max int32", pidText: strconv.FormatInt(math.MaxInt32, 10), known: true, wantPID: math.MaxInt32},
		{name: "max int32 plus one", pidText: "2147483648"},
		{name: "uint32 boundary", pidText: "4294967296"},
		{name: "strconv overflow", pidText: "18446744073709551616"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "start.lock")
			body := "version=1\npid=" + test.pidText + "\nnonce=00112233445566778899aabbccddeeff\n"
			writeRawStartLock(t, path, body)
			called := false
			reclaimed, err := tryReclaimStartLock(path, time.Now(), func(pid int) bool {
				called = true
				if !test.known || pid != test.wantPID {
					t.Fatalf("ownerAlive called with pid %d", pid)
				}
				return true
			})
			if err != nil {
				t.Fatalf("tryReclaimStartLock: %v", err)
			}
			if reclaimed {
				t.Fatal("start.lock was reclaimed")
			}
			if called != test.known {
				t.Fatalf("ownerAlive called = %v, want %v", called, test.known)
			}
			assertStartLockBody(t, path, body)
		})
	}
}

func TestAcquireStartLockReclaimsDeadOwnerAndCreatesReplacementUnderGuard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.lock")
	writeRawStartLock(t, path, "version=1\npid=4242\nnonce=00112233445566778899aabbccddeeff\n")

	lock, err := acquireStartLockAtWithNonce(path, time.Second, time.Now, func(pid int) bool { return pid != 4242 }, func() (string, error) {
		return "ffeeddccbbaa99887766554433221100", nil
	})
	if err != nil {
		t.Fatalf("acquireStartLockAtWithNonce: %v", err)
	}
	defer lock.Close()
	assertStartLockBody(t, path, "version=1\npid="+itoa(os.Getpid())+"\nnonce=ffeeddccbbaa99887766554433221100\n")
}

func TestReclaimStartLockUnderGuardPreservesLiveAndUnknownOwners(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		alive func(int) bool
	}{
		{
			name:  "live",
			body:  "version=1\npid=4242\nnonce=00112233445566778899aabbccddeeff\n",
			alive: func(pid int) bool { return pid == 4242 },
		},
		{
			name:  "unknown",
			body:  "version=99\npid=4242\nnonce=00112233445566778899aabbccddeeff\n",
			alive: func(int) bool { return false },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "start.lock")
			writeRawStartLock(t, path, test.body)
			reclaimed, err := tryReclaimStartLock(path, time.Now(), test.alive)
			if err != nil {
				t.Fatalf("tryReclaimStartLock: %v", err)
			}
			if reclaimed {
				t.Fatal("owner was reclaimed")
			}
			assertStartLockBody(t, path, test.body)
		})
	}
}

func TestReclaimStartLockLegacyAge(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, test := range []struct {
		name      string
		age       time.Duration
		reclaimed bool
	}{
		{name: "recent", age: startLockLegacyStaleAfter - time.Second, reclaimed: false},
		{name: "old", age: startLockLegacyStaleAfter + time.Second, reclaimed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "start.lock")
			writeRawStartLock(t, path, "")
			mtime := now.Add(-test.age)
			if err := os.Chtimes(path, mtime, mtime); err != nil {
				t.Fatal(err)
			}
			reclaimed, err := tryReclaimStartLock(path, now, func(int) bool { return false })
			if err != nil {
				t.Fatalf("tryReclaimStartLock: %v", err)
			}
			if reclaimed != test.reclaimed {
				t.Fatalf("reclaimed = %v, want %v", reclaimed, test.reclaimed)
			}
			if test.reclaimed {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("legacy lock still exists, stat error = %v", err)
				}
			} else {
				assertStartLockBody(t, path, "")
			}
		})
	}
}

func TestStartLockCloseDoesNotDeleteNonceReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.lock")
	old, err := acquireStartLockAtWithNonce(path, time.Second, time.Now, func(int) bool { return true }, func() (string, error) {
		return "00112233445566778899aabbccddeeff", nil
	})
	if err != nil {
		t.Fatalf("old acquire: %v", err)
	}
	writeRawStartLock(t, path, "version=1\npid="+itoa(old.pid)+"\nnonce=ffeeddccbbaa99887766554433221100\n")
	if err := old.Close(); err != nil {
		t.Fatalf("old Close: %v", err)
	}
	assertStartLockBody(t, path, "version=1\npid="+itoa(old.pid)+"\nnonce=ffeeddccbbaa99887766554433221100\n")
}

func TestStartLockConcurrentAcquireReclaimAndClosePreservesReplacement(t *testing.T) {
	for iteration := 0; iteration < 4; iteration++ {
		t.Run(itoa(iteration), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "start.lock")
			old, err := acquireStartLockAt(path, time.Second, time.Now, func(int) bool { return true })
			if err != nil {
				t.Fatalf("old acquire: %v", err)
			}
			started := make(chan struct{})
			var once sync.Once
			var replacement *startLock
			var replacementErr error
			var closeErr error
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				once.Do(func() { close(started) })
				closeErr = old.Close()
			}()
			go func() {
				defer wg.Done()
				<-started
				replacement, replacementErr = acquireStartLockAt(path, time.Second, time.Now, func(int) bool { return false })
			}()
			wg.Wait()
			if closeErr != nil {
				t.Fatalf("old Close: %v", closeErr)
			}
			if replacementErr != nil {
				t.Fatalf("replacement acquire: %v", replacementErr)
			}
			assertStartLockBody(t, path, "version=1\npid="+itoa(replacement.pid)+"\nnonce="+replacement.nonce+"\n")
			if err := replacement.Close(); err != nil {
				t.Fatalf("replacement Close: %v", err)
			}
		})
	}
}

func TestStartLockConcurrentAcquiresHaveSingleOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.lock")
	start := make(chan struct{})
	results := make(chan *startLock, 2)
	errorsCh := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			lock, err := acquireStartLockAt(path, 100*time.Millisecond, time.Now, func(int) bool { return true })
			if err != nil {
				errorsCh <- err
				return
			}
			results <- lock
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errorsCh)

	var owners []*startLock
	for lock := range results {
		owners = append(owners, lock)
	}
	if len(owners) != 1 {
		t.Fatalf("successful concurrent acquires = %d, want 1", len(owners))
	}
	for err := range errorsCh {
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("losing acquire error = %v, want timeout", err)
		}
	}
	if err := owners[0].Close(); err != nil {
		t.Fatalf("owner Close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("start.lock remains after Close, stat error = %v", err)
	}
}

func TestStartLockGuardIsPersistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.lock")
	lock, err := acquireStartLockAt(path, time.Second, time.Now, func(int) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "start.guard")); err != nil {
		t.Fatalf("start.guard missing after Close: %v", err)
	}
}

func tryReclaimStartLock(path string, now time.Time, ownerAlive func(int) bool) (bool, error) {
	if now.IsZero() {
		now = time.Now()
	}
	var reclaimed bool
	err := withStartGuard(filepath.Join(filepath.Dir(path), "start.guard"), time.Second, func() time.Time { return now }, func() error {
		var err error
		reclaimed, err = reclaimStartLockLocked(path, now, ownerAlive)
		return err
	})
	return reclaimed, err
}

func writeRawStartLock(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertStartLockBody(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != want {
		t.Fatalf("start.lock = %q, want %q", body, want)
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
