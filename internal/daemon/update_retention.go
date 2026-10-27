package daemon

import (
	"sync"
	"time"
)

// updateRetention owns and joins the periodic telemetry producer before the
// telemetry store is closed. A running purge holds the admission lease.
type updateRetention struct {
	gate  *updateAdmissionGate
	purge func()
	stop  chan struct{}
	done  chan struct{}
	tick  chan struct{}
	once  sync.Once
}

func newUpdateRetention(gate *updateAdmissionGate, purge func()) *updateRetention {
	r := &updateRetention{gate: gate, purge: purge, stop: make(chan struct{}), done: make(chan struct{}), tick: make(chan struct{}, 1)}
	go r.run()
	return r
}
func (r *updateRetention) run() {
	defer close(r.done)
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.runPurge()
		case <-r.tick:
			r.runPurge()
		}
	}
}
func (r *updateRetention) runPurge() {
	end, ok := r.gate.Begin("telemetry-retention")
	if !ok {
		return
	}
	defer end()
	r.purge()
}
func (r *updateRetention) startForTest() { r.tick <- struct{}{} }
func (r *updateRetention) stopAndWait()  { r.once.Do(func() { close(r.stop) }); <-r.done }
