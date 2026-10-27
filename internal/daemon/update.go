package daemon

import (
	"context"
	"sync"
)

// Admission tracks all activities registered by the daemon. An idle barrier
// excludes new admissions until released; it is not by itself proof that
// uninstrumented background activity or external sessions are idle.
type updateAdmissionGate struct {
	mu      sync.Mutex
	active  int
	barrier bool
}

func newUpdateAdmissionGate() *updateAdmissionGate { return &updateAdmissionGate{} }
func (g *updateAdmissionGate) Begin(_ string) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.barrier {
		return nil, false
	}
	g.active++
	var once sync.Once
	return func() { once.Do(func() { g.mu.Lock(); g.active--; g.mu.Unlock() }) }, true
}
func (g *updateAdmissionGate) TryIdleBarrier() (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.barrier || g.active != 0 {
		return nil, false
	}
	g.barrier = true
	var once sync.Once
	return func() { once.Do(func() { g.mu.Lock(); g.barrier = false; g.mu.Unlock() }) }, true
}

type updateRestartInvocation struct {
	Executable string
	Args       []string
	Env        []string
	Endpoint   string
	HomeDir    string
}
type updateRestartOps struct {
	CleanupOld func() error
	Shutdown   func() error
	Spawn      func(updateRestartInvocation) error
}

func restartUpdatedDaemon(ctx context.Context, invocation updateRestartInvocation, ops updateRestartOps) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ops.CleanupOld(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ops.Shutdown(); err != nil {
		return err
	}
	return ops.Spawn(invocation)
}
