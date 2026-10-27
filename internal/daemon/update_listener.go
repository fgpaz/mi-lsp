package daemon

import (
	"context"
	"errors"
	"net"
	"sync"
)

// updateListener pauses only the listening socket. Connections already returned
// by Accept retain an active lease until their owner closes them.
type updateListener struct {
	mu                                 sync.Mutex
	inner                              net.Listener
	reopen                             func() (net.Listener, error)
	addr                               net.Addr
	changed                            chan struct{}
	paused, closed, pausing, reopening bool
	generation                         uint64
	pending, active                    int
}

func newUpdateListener(inner net.Listener, reopen func() (net.Listener, error)) *updateListener {
	return &updateListener{inner: inner, reopen: reopen, addr: inner.Addr(), changed: make(chan struct{})}
}

// signalLocked broadcasts changes without retaining a goroutine per waiter.
func (l *updateListener) signalLocked()  { close(l.changed); l.changed = make(chan struct{}) }
func (l *updateListener) Addr() net.Addr { return l.addr }
func (l *updateListener) Accept() (net.Conn, error) {
	for {
		l.mu.Lock()
		for !l.closed && (l.paused || l.inner == nil) {
			ch := l.changed
			l.mu.Unlock()
			<-ch
			l.mu.Lock()
		}
		if l.closed {
			l.mu.Unlock()
			return nil, net.ErrClosed
		}
		inner, gen := l.inner, l.generation
		l.pending++
		l.mu.Unlock()
		conn, err := inner.Accept()
		l.mu.Lock()
		// A delivered socket is leased before pending is decremented, even if Pause
		// or Close won the race with the underlying Accept.
		if conn != nil {
			l.active++
		}
		l.pending--
		closed, paused, changed := l.closed, l.paused, gen != l.generation
		l.signalLocked()
		l.mu.Unlock()
		if conn != nil {
			return &updateLease{Conn: conn, owner: l}, err
		}
		if closed {
			return nil, net.ErrClosed
		}
		if paused || changed {
			continue
		}
		return nil, err
	}
}
func (l *updateListener) Pause(ctx context.Context) (bool, error) {
	l.mu.Lock()
	for l.pausing && !l.closed {
		ch := l.changed
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ch:
		}
		l.mu.Lock()
	}
	if l.closed {
		l.mu.Unlock()
		return false, net.ErrClosed
	}
	l.pausing = true
	var inner net.Listener
	l.generation++
	if !l.paused {
		l.paused = true
		inner = l.inner
		l.inner = nil
	}
	l.signalLocked()
	l.mu.Unlock()
	var closeErr error
	if inner != nil {
		closeErr = inner.Close()
	}
	l.mu.Lock()
	defer func() { l.pausing = false; l.signalLocked(); l.mu.Unlock() }()
	for l.pending > 0 && !l.closed {
		ch := l.changed
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			l.mu.Lock()
			return false, ctx.Err()
		case <-ch:
		}
		l.mu.Lock()
	}
	if l.closed {
		return false, net.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
		return false, closeErr
	}
	return l.active == 0, nil
}
func (l *updateListener) Resume() error {
	l.mu.Lock()
	for (l.pausing || l.reopening) && !l.closed {
		ch := l.changed
		l.mu.Unlock()
		<-ch
		l.mu.Lock()
	}
	if l.closed {
		l.mu.Unlock()
		return net.ErrClosed
	}
	if !l.paused {
		l.mu.Unlock()
		return nil
	}
	if l.reopen == nil {
		l.mu.Unlock()
		return errors.New("listener reopen unavailable")
	}
	l.reopening = true
	gen := l.generation
	factory := l.reopen
	l.mu.Unlock()
	inner, err := factory()
	l.mu.Lock()
	l.reopening = false
	if l.closed {
		l.signalLocked()
		l.mu.Unlock()
		if inner != nil {
			_ = inner.Close()
		}
		return net.ErrClosed
	}
	if gen != l.generation || l.pausing {
		l.signalLocked()
		l.mu.Unlock()
		if inner != nil {
			_ = inner.Close()
		}
		return errors.New("listener pause changed during reopen")
	}
	if err != nil {
		l.signalLocked()
		l.mu.Unlock()
		if inner != nil {
			_ = inner.Close()
		}
		return err
	}
	if inner == nil {
		l.signalLocked()
		l.mu.Unlock()
		return errors.New("listener reopen returned nil")
	}
	l.inner = inner
	l.paused = false
	l.signalLocked()
	l.mu.Unlock()
	return nil
}
func (l *updateListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	l.generation++
	inner := l.inner
	l.inner = nil
	l.signalLocked()
	l.mu.Unlock()
	if inner != nil {
		err := inner.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
	}
	return nil
}

type updateLease struct {
	net.Conn
	owner *updateListener
	once  sync.Once
	err   error
}

func (c *updateLease) Close() error {
	c.once.Do(func() {
		c.err = c.Conn.Close()
		c.owner.mu.Lock()
		c.owner.active--
		c.owner.signalLocked()
		c.owner.mu.Unlock()
	})
	return c.err
}
