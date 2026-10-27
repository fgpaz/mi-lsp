package daemon

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

// controlledListener exposes the interval after inner.Accept has acquired a
// connection but before it returns it to updateListener. Close only closes the
// listening socket; it must never close a connection already accepted.
type controlledListener struct {
	entered  chan struct{}
	closed   chan struct{}
	offer    chan net.Conn
	release  chan struct{}
	acquired chan struct{}
	once     sync.Once
}

func newControlledListener() *controlledListener {
	return &controlledListener{entered: make(chan struct{}, 1), closed: make(chan struct{}), offer: make(chan net.Conn), acquired: make(chan struct{}, 1)}
}
func (l *controlledListener) Accept() (net.Conn, error) {
	select {
	case l.entered <- struct{}{}:
	default:
	}
	select {
	case c := <-l.offer:
		if l.release != nil {
			l.acquired <- struct{}{}
			<-l.release
		}
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *controlledListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *controlledListener) Addr() net.Addr { return listenerTestAddr("fixture") }

type listenerTestAddr string

func (a listenerTestAddr) Network() string { return "fixture" }
func (a listenerTestAddr) String() string  { return string(a) }

type acceptResult struct {
	conn net.Conn
	err  error
}
type pauseResult struct {
	idle bool
	err  error
}

func awaitEvent(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for listener event")
	}
}
func awaitAccept(t *testing.T, ch <-chan acceptResult) acceptResult {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Accept")
		return acceptResult{}
	}
}
func awaitPause(t *testing.T, ch <-chan pauseResult) pauseResult {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Pause")
		return pauseResult{}
	}
}
func startAccept(l net.Listener) <-chan acceptResult {
	ch := make(chan acceptResult, 1)
	go func() { c, e := l.Accept(); ch <- acceptResult{c, e} }()
	return ch
}
func startPause(l *updateListener, ctx context.Context) <-chan pauseResult {
	ch := make(chan pauseResult, 1)
	go func() { idle, e := l.Pause(ctx); ch <- pauseResult{idle, e} }()
	return ch
}
func listenerPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	a, b := net.Pipe()
	deadline := time.Now().Add(2 * time.Second)
	if err := a.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := b.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	return a, b
}
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestUpdateListenerAcceptedBeforeRegistration(t *testing.T) {
	inner := newControlledListener()
	inner.release = make(chan struct{})
	replacement := newControlledListener()
	l := newUpdateListener(inner, func() (net.Listener, error) { return replacement, nil })
	t.Cleanup(func() { _ = l.Close() })
	results := startAccept(l)
	awaitEvent(t, inner.entered)
	server, client := listenerPipe(t)
	t.Cleanup(func() { _ = server.Close(); _ = client.Close() })
	sent := make(chan struct{})
	go func() { inner.offer <- server; close(sent) }()
	awaitEvent(t, sent)
	awaitEvent(t, inner.acquired)
	paused := startPause(l, testContext(t))
	awaitEvent(t, inner.closed)
	select {
	case r := <-paused:
		t.Fatalf("Pause returned before pending acceptance settled: %+v", r)
	default:
	}
	close(inner.release)
	accepted := awaitAccept(t, results)
	if accepted.err != nil || accepted.conn == nil {
		t.Fatalf("accepted=%+v", accepted)
	}
	t.Cleanup(func() { _ = accepted.conn.Close() })
	p := awaitPause(t, paused)
	if p.err != nil || p.idle {
		t.Fatalf("active connection reported idle: %+v", p)
	}
	if err := l.Resume(); err != nil {
		t.Fatal(err)
	}
	// A real pending response must survive Pause and Resume.
	reply := make(chan error, 1)
	go func() { _, e := accepted.conn.Write([]byte("ok")); reply <- e }()
	buf := make([]byte, 2)
	if _, e := client.Read(buf); e != nil || string(buf) != "ok" {
		t.Fatalf("response=%q err=%v", buf, e)
	}
	select {
	case e := <-reply:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("response writer stuck")
	}
	if err := accepted.conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := accepted.conn.Close(); err != nil {
		t.Fatal(err)
	}
	p = awaitPause(t, startPause(l, testContext(t)))
	if p.err != nil || !p.idle {
		t.Fatalf("closed lease must be idle: %+v", p)
	}
}
func TestUpdateListenerPauseEmptyResumeAndClose(t *testing.T) {
	inner := newControlledListener()
	replacement := newControlledListener()
	l := newUpdateListener(inner, func() (net.Listener, error) { return replacement, nil })
	t.Cleanup(func() { _ = l.Close() })
	results := startAccept(l)
	awaitEvent(t, inner.entered)
	p := awaitPause(t, startPause(l, testContext(t)))
	if p.err != nil || !p.idle {
		t.Fatalf("empty listener: %+v", p)
	}
	awaitEvent(t, inner.closed)
	select {
	case r := <-results:
		t.Fatalf("interrupted Accept returned while paused: %+v", r)
	default:
	}
	if err := l.Resume(); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, replacement.entered)
	server, client := listenerPipe(t)
	defer client.Close()
	sent := make(chan struct{})
	go func() { replacement.offer <- server; close(sent) }()
	awaitEvent(t, sent)
	accepted := awaitAccept(t, results)
	if accepted.err != nil || accepted.conn == nil {
		t.Fatalf("accepted=%+v", accepted)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("second Close: %v", err)
	}
	// Closing a listener does not close accepted connections.
	done := make(chan error, 1)
	go func() { _, e := accepted.conn.Write([]byte("x")); done <- e }()
	b := make([]byte, 1)
	if _, err := client.Read(b); err != nil || string(b) != "x" {
		t.Fatalf("accepted connection lost: %q %v", b, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write stuck")
	}
	_ = accepted.conn.Close()
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after Close: %v", err)
	}
}
func TestUpdateListenerCloseWakesPausedAccept(t *testing.T) {
	inner := newControlledListener()
	l := newUpdateListener(inner, func() (net.Listener, error) { return newControlledListener(), nil })
	results := startAccept(l)
	awaitEvent(t, inner.entered)
	p := awaitPause(t, startPause(l, testContext(t)))
	if p.err != nil || !p.idle {
		t.Fatalf("pause=%+v", p)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	r := awaitAccept(t, results)
	if !errors.Is(r.err, net.ErrClosed) || r.conn != nil {
		t.Fatalf("accept=%+v", r)
	}
	if err := l.Resume(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Resume closed listener: %v", err)
	}
}
func TestUpdateListenerPauseContextCancelAndResume(t *testing.T) {
	inner := newControlledListener()
	inner.release = make(chan struct{})
	next := newControlledListener()
	l := newUpdateListener(inner, func() (net.Listener, error) { return next, nil })
	t.Cleanup(func() { _ = l.Close() })
	results := startAccept(l)
	awaitEvent(t, inner.entered)
	server, client := listenerPipe(t)
	defer client.Close()
	sent := make(chan struct{})
	go func() { inner.offer <- server; close(sent) }()
	awaitEvent(t, sent)
	awaitEvent(t, inner.acquired)
	ctx, cancel := context.WithCancel(context.Background())
	paused := startPause(l, ctx)
	awaitEvent(t, inner.closed)
	cancel()
	p := awaitPause(t, paused)
	if p.idle || !errors.Is(p.err, context.Canceled) {
		t.Fatalf("pause cancellation=%+v", p)
	}
	close(inner.release)
	r := awaitAccept(t, results)
	if r.err != nil || r.conn == nil {
		t.Fatalf("accepted=%+v", r)
	}
	defer r.conn.Close()
	if err := l.Resume(); err != nil {
		t.Fatal(err)
	}
	again := startAccept(l)
	awaitEvent(t, next.entered)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	r = awaitAccept(t, again)
	if !errors.Is(r.err, net.ErrClosed) {
		t.Fatalf("accept after close=%+v", r)
	}
}
func TestUpdateListenerReopenFailureAndClose(t *testing.T) {
	inner := newControlledListener()
	failure := errors.New("fixture reopen failure")
	l := newUpdateListener(inner, func() (net.Listener, error) { return nil, failure })
	t.Cleanup(func() { _ = l.Close() })
	results := startAccept(l)
	awaitEvent(t, inner.entered)
	p := awaitPause(t, startPause(l, testContext(t)))
	if p.err != nil || !p.idle {
		t.Fatalf("pause=%+v", p)
	}
	if err := l.Resume(); !errors.Is(err, failure) {
		t.Fatalf("Resume error=%v", err)
	}
	select {
	case r := <-results:
		t.Fatalf("Accept exited on reopen failure: %+v", r)
	default:
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	r := awaitAccept(t, results)
	if !errors.Is(r.err, net.ErrClosed) {
		t.Fatalf("accept=%+v", r)
	}
}
func TestUpdateListenerCloseDuringReopen(t *testing.T) {
	inner := newControlledListener()
	replacement := newControlledListener()
	entered := make(chan struct{})
	release := make(chan struct{})
	l := newUpdateListener(inner, func() (net.Listener, error) { close(entered); <-release; return replacement, nil })
	p := awaitPause(t, startPause(l, testContext(t)))
	if p.err != nil || !p.idle {
		t.Fatalf("pause=%+v", p)
	}
	done := make(chan error, 1)
	go func() { done <- l.Resume() }()
	awaitEvent(t, entered)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Resume after Close=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Resume stuck")
	}
	awaitEvent(t, replacement.closed)
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed Accept=%v", err)
	}
}
