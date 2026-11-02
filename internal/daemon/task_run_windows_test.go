//go:build windows
// +build windows

package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

const taskRunTestWatchdog = 5 * time.Second
const testTaskRunHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testTaskRunNonce = "0123456789abcdef0123456789abcdef"
const testTaskRunGUID = "{12345678-1234-1234-1234-123456789abc}"

func testRequest() taskRunRequest {
	return taskRunRequest{TaskPath: `\Vendor\Task`, ExpectedXMLSHA256: testTaskRunHash, Nonce: testTaskRunNonce}
}

type fakeTaskRunSession struct {
	preflight func(context.Context, taskRunRequest) error
	prepare   func(context.Context) error
	run       func(string) (string, error)
	closed    chan struct{}
	closeErr  error
}

func (f *fakeTaskRunSession) Preflight(ctx context.Context, r taskRunRequest) error {
	if f.preflight != nil { return f.preflight(ctx, r) }
	return nil
}
func (f *fakeTaskRunSession) PrepareRun(ctx context.Context) error {
	if f.prepare != nil { return f.prepare(ctx) }
	return nil
}
func (f *fakeTaskRunSession) Run(nonce string) (string, error) { return f.run(nonce) }
func (f *fakeTaskRunSession) Close() error {
	if f.closed != nil { close(f.closed) }
	return f.closeErr
}

func waitTaskRunSignal(t testing.TB, ch <-chan struct{}, label string) {
	t.Helper()
	timer := time.NewTimer(taskRunTestWatchdog)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("watchdog waiting for %s", label)
	}
}

func waitTaskRunResult(t testing.TB, ch <-chan taskRunResult) taskRunResult {
	t.Helper()
	timer := time.NewTimer(taskRunTestWatchdog)
	defer timer.Stop()
	select {
	case result := <-ch:
		return result
	case <-timer.C:
		t.Fatalf("watchdog waiting for Task Run result")
		return taskRunResult{}
	}
}

func waitTaskRunString(t testing.TB, ch <-chan string, label string) string {
	t.Helper()
	timer := time.NewTimer(taskRunTestWatchdog)
	defer timer.Stop()
	select {
	case value := <-ch:
		return value
	case <-timer.C:
		t.Fatalf("watchdog waiting for %s", label)
		return ""
	}
}

func testTaskRunContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), taskRunTestWatchdog)
}

func TestTaskRunInitiallyCanceledOrExpiredDoesNotCreateSession(t *testing.T) {
	for _, tc := range []struct { name string; ctx context.Context; cancel context.CancelFunc; want error }{
		{name: "canceled", ctx: func() context.Context { c, cancel := context.WithCancel(context.Background()); cancel(); return c }(), want: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factoryCalled := false
			got := invokeTaskRunWithFactory(tc.ctx, testRequest(), func() (taskRunSession, error) {
				factoryCalled = true
				return nil, errors.New("unexpected factory call")
			})
			if got.Phase != taskRunNotEntered || !errors.Is(got.Err, tc.want) || factoryCalled {
				t.Fatalf("result = %#v, factoryCalled = %v", got, factoryCalled)
			}
			if tc.cancel != nil { tc.cancel() }
		})
	}
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	factoryCalled := false
	got := invokeTaskRunWithFactory(expired, testRequest(), func() (taskRunSession, error) {
		factoryCalled = true
		return nil, errors.New("unexpected factory call")
	})
	if got.Phase != taskRunNotEntered || !errors.Is(got.Err, context.DeadlineExceeded) || factoryCalled {
		t.Fatalf("expired result = %#v, factoryCalled = %v", got, factoryCalled)
	}
}

func TestTaskRunCancelWhileFactoryLatchedClosesReturnedSessionWithoutNextStage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	factoryEntered, releaseFactory := make(chan struct{}), make(chan struct{})
	preflightCalled, prepareCalled, runCalled := make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{}, 1)
	closed := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFactory) }) }
	defer func() { cancel(); release() }()
	session := &fakeTaskRunSession{
		preflight: func(context.Context, taskRunRequest) error { preflightCalled <- struct{}{}; return nil },
		prepare: func(context.Context) error { prepareCalled <- struct{}{}; return nil },
		run: func(string) (string, error) { runCalled <- struct{}{}; return testTaskRunGUID, nil },
		closed: closed,
	}
	result := invokeTaskRunWithFactoryAsync(ctx, testRequest(), func() (taskRunSession, error) {
		close(factoryEntered); <-releaseFactory; return session, nil
	})
	waitTaskRunSignal(t, factoryEntered, "factory entry")
	cancel()
	got := waitTaskRunResult(t, result)
	if got.Phase != taskRunNotEntered { t.Fatalf("phase = %v, want NOT_ENTERED", got.Phase) }
	release()
	waitTaskRunSignal(t, closed, "returned session Close")
	for label, ch := range map[string]<-chan struct{}{"Preflight": preflightCalled, "PrepareRun": prepareCalled, "Run": runCalled} {
		select { case <-ch: t.Errorf("%s called after cancellation at factory", label); default: }
	}
}

func TestTaskRunCanceledBeforeEntryNeverRunsLate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	preflightEntered, allowPreflight, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(allowPreflight) }) }
	defer func() { cancel(); release() }() // always unstrand the fake worker
	runCalled := make(chan struct{}, 1)
	session := &fakeTaskRunSession{
		preflight: func(context.Context, taskRunRequest) error { close(preflightEntered); <-allowPreflight; return nil },
		prepare: func(context.Context) error { t.Error("PrepareRun called after cancellation during Preflight"); return nil },
		run: func(string) (string, error) { runCalled <- struct{}{}; return testTaskRunGUID, nil },
		closed: closed,
	}
	result := invokeTaskRunWithFactoryAsync(ctx, testRequest(), func() (taskRunSession, error) { return session, nil })
	waitTaskRunSignal(t, preflightEntered, "Preflight entry")
	cancel()
	got := waitTaskRunResult(t, result)
	if got.Phase != taskRunNotEntered { t.Fatalf("phase = %v, want NOT_ENTERED", got.Phase) }
	release()
	waitTaskRunSignal(t, closed, "fake session Close")
	select {
	case <-runCalled:
		t.Fatal("late preflight entered Run after NOT_ENTERED")
	default:
	}
}

func TestTaskRunCancelWhilePrepareLatchedSkipsRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	prepareEntered, releasePrepare, runCalled, closed := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePrepare) }) }
	defer func() { cancel(); release() }()
	session := &fakeTaskRunSession{
		prepare: func(context.Context) error { close(prepareEntered); <-releasePrepare; return nil },
		run: func(string) (string, error) { runCalled <- struct{}{}; return testTaskRunGUID, nil },
		closed: closed,
	}
	result := invokeTaskRunWithFactoryAsync(ctx, testRequest(), func() (taskRunSession, error) { return session, nil })
	waitTaskRunSignal(t, prepareEntered, "PrepareRun entry")
	cancel()
	got := waitTaskRunResult(t, result)
	if got.Phase != taskRunNotEntered { t.Fatalf("phase = %v, want NOT_ENTERED", got.Phase) }
	release()
	waitTaskRunSignal(t, closed, "fake session Close")
	select { case <-runCalled: t.Fatal("Run called after cancellation during PrepareRun"); default: }
}

func TestTaskRunCancellationAfterEntryIsUncertainAndSingleRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered, releaseRun, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRun) }) }
	defer func() { cancel(); release() }() // release Run even if a watchdog fails
	runCalls := make(chan string, 2)
	session := &fakeTaskRunSession{
		run: func(nonce string) (string, error) { runCalls <- nonce; close(entered); <-releaseRun; return testTaskRunGUID, nil },
		closed: closed,
	}
	result := invokeTaskRunWithFactoryAsync(ctx, testRequest(), func() (taskRunSession, error) { return session, nil })
	waitTaskRunSignal(t, entered, "Run entry")
	cancel()
	got := waitTaskRunResult(t, result)
	if got.Phase != taskRunUncertain || got.InstanceGUID != "" { t.Fatalf("result = %#v, want UNCERTAIN without GUID", got) }
	release()
	waitTaskRunSignal(t, closed, "fake session Close")
	if nonce := waitTaskRunString(t, runCalls, "Run nonce"); nonce != testTaskRunNonce { t.Fatalf("nonce = %q", nonce) }
	select {
	case <-runCalls:
		t.Fatal("Run called more than once")
	default:
	}
}

func TestTaskRunSuccessIsOnlyGUIDKnown(t *testing.T) {
	ctx, cancel := testTaskRunContext(t)
	defer cancel()
	closed := make(chan struct{})
	session := &fakeTaskRunSession{closed: closed, run: func(nonce string) (string, error) {
		if nonce != testTaskRunNonce { t.Fatalf("nonce = %q", nonce) }
		return testTaskRunGUID, nil
	}}
	got := invokeTaskRunWithFactory(ctx, testRequest(), func() (taskRunSession, error) { return session, nil })
	if got.Phase != taskRunGUIDKnown || got.InstanceGUID != testTaskRunGUID || got.Err != nil {
		t.Fatalf("result = %#v, want GUID_KNOWN only", got)
	}
	select { case <-closed: default: t.Fatal("session cleanup did not complete before return") }
}

func TestTaskRunPreflightAndPrepareErrorsDoNotEnter(t *testing.T) {
	for _, tc := range []struct { name string; preflight, prepare error }{
		{name: "preflight", preflight: errors.New("registered XML mismatch")},
		{name: "dispatch lookup", prepare: errors.New("GetIDsOfNames failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := testTaskRunContext(t)
			defer cancel()
			runCalled := false
			session := &fakeTaskRunSession{
				preflight: func(context.Context, taskRunRequest) error { return tc.preflight },
				prepare: func(context.Context) error { return tc.prepare },
				run: func(string) (string, error) { runCalled = true; return testTaskRunGUID, nil },
			}
			got := invokeTaskRunWithFactory(ctx, testRequest(), func() (taskRunSession, error) { return session, nil })
			wantErr := tc.preflight
			if wantErr == nil { wantErr = tc.prepare }
			if got.Phase != taskRunNotEntered || !errors.Is(got.Err, wantErr) || runCalled {
				t.Fatalf("result = %#v, runCalled = %v", got, runCalled)
			}
		})
	}
}

func TestTaskRunPostEntryErrorOrInvalidGUIDIsUncertain(t *testing.T) {
	for _, tc := range []struct { name, guid string; err error }{
		{name: "COM error", err: errors.New("Run failed after entry")},
		{name: "invalid GUID", guid: "{00000000-0000-0000-0000-000000000000}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := testTaskRunContext(t)
			defer cancel()
			session := &fakeTaskRunSession{run: func(string) (string, error) { return tc.guid, tc.err }}
			got := invokeTaskRunWithFactory(ctx, testRequest(), func() (taskRunSession, error) { return session, nil })
			if got.Phase != taskRunUncertain || got.InstanceGUID != "" { t.Fatalf("result = %#v, want UNCERTAIN without GUID", got) }
		})
	}
}

func TestTaskRunCloseErrorInvalidatesKnownGUID(t *testing.T) {
	ctx, cancel := testTaskRunContext(t)
	defer cancel()
	closeErr := errors.New("fake Close failed")
	session := &fakeTaskRunSession{closeErr: closeErr, run: func(string) (string, error) { return testTaskRunGUID, nil }}
	got := invokeTaskRunWithFactory(ctx, testRequest(), func() (taskRunSession, error) { return session, nil })
	if got.Phase != taskRunUncertain || got.InstanceGUID != "" || !errors.Is(got.Err, closeErr) {
		t.Fatalf("result = %#v, want cleanup error and UNCERTAIN", got)
	}
}

func TestTaskRunCleanupJoinSurfacesAllErrors(t *testing.T) {
	primary, first, second := errors.New("primary"), errors.New("clear one"), errors.New("clear two")
	calls := 0
	got := joinTaskRunCleanup(primary,
		func() error { calls++; return first },
		func() error { calls++; return second },
	)
	if calls != 2 || !errors.Is(got, primary) || !errors.Is(got, first) || !errors.Is(got, second) {
		t.Fatalf("cleanup result = %v; calls = %d", got, calls)
	}
}

func TestTaskRunMalformedRequestAndGUID(t *testing.T) {
	for _, taskPath := range []string{"", "relative\\Task", "\\Vendor\\..\\Task", "\\Vendor\\Task\\"} {
		if validTaskSchedulerPath(taskPath) { t.Errorf("accepted invalid Task path %q", taskPath) }
	}
	ctx, cancel := testTaskRunContext(t)
	defer cancel()
	bad := testRequest()
	bad.Nonce = strings.ToUpper(bad.Nonce)
	called := false
	factory := func() (taskRunSession, error) { called = true; return nil, errors.New("must not create session") }
	if got := invokeTaskRunWithFactory(ctx, bad, factory); got.Phase != taskRunNotEntered || called {
		t.Fatalf("malformed request = %#v, factory called = %v", got, called)
	}
	for _, guid := range []string{"", "{00000000-0000-0000-0000-000000000000}", "not-a-guid", "{12345678-1234-1234-1234-123456789abz}"} {
		if validTaskInstanceGUID(guid) { t.Errorf("accepted invalid GUID %q", guid) }
	}
	if !validTaskInstanceGUID(testTaskRunGUID) { t.Fatal("rejected valid GUID") }
}

func invokeTaskRunWithFactoryAsync(ctx context.Context, request taskRunRequest, factory taskRunSessionFactory) <-chan taskRunResult {
	result := make(chan taskRunResult, 1)
	go func() { result <- invokeTaskRunWithFactory(ctx, request, factory) }()
	return result
}
