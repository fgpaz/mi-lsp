package daemon

import (
	"context"
	"net"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func (s *Server) registerIngressConnection(conn net.Conn) bool {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	if s.shutdownStarted.Load() {
		return false
	}
	if s.connections == nil {
		s.connections = make(map[net.Conn]struct{})
	}
	if s.connectionCancels == nil {
		s.connectionCancels = make(map[net.Conn]context.CancelFunc)
	}
	if s.semanticConnections == nil {
		s.semanticConnections = make(map[net.Conn]struct{})
	}
	s.connections[conn] = struct{}{}
	return true
}

func (s *Server) unregisterIngressConnection(conn net.Conn) {
	s.connectionsMu.Lock()
	delete(s.connections, conn)
	delete(s.connectionCancels, conn)
	delete(s.semanticConnections, conn)
	s.connectionsMu.Unlock()
	select {
	case s.connectionsChanged <- struct{}{}:
	default:
	}
	s.signalShutdownRetry()
}

func (s *Server) waitForIngressHandlers() bool {
	for {
		s.connectionsMu.Lock()
		active := len(s.connections)
		s.connectionsMu.Unlock()
		if active == 0 {
			return true
		}
		if s.connectionsChanged == nil {
			return false
		}
		<-s.connectionsChanged
	}
}

func (s *Server) trackConnectionHandler(conn net.Conn, cancel context.CancelFunc, semantic bool) bool {
	s.workMu.Lock()
	defer s.workMu.Unlock()
	if s.shutdownStarted.Load() {
		return false
	}
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	if s.shutdownStarted.Load() {
		return false
	}
	if s.connectionCancels == nil {
		s.connectionCancels = make(map[net.Conn]context.CancelFunc)
	}
	if s.semanticConnections == nil {
		s.semanticConnections = make(map[net.Conn]struct{})
	}
	s.connectionCancels[conn] = cancel
	if semantic {
		s.semanticConnections[conn] = struct{}{}
		s.activeWork++
		select {
		case s.workChanged <- struct{}{}:
		default:
		}
	}
	return true
}

func (s *Server) detachConnectionCancel(conn net.Conn) {
	s.connectionsMu.Lock()
	delete(s.connectionCancels, conn)
	delete(s.semanticConnections, conn)
	s.connectionsMu.Unlock()
}

func (s *Server) closeIdleIngressConnections() {
	s.connectionsMu.Lock()
	connections := make([]net.Conn, 0, len(s.connections))
	cancels := make([]context.CancelFunc, 0, len(s.connectionCancels))
	for conn := range s.connections {
		if _, semantic := s.semanticConnections[conn]; !semantic {
			connections = append(connections, conn)
			if cancel := s.connectionCancels[conn]; cancel != nil {
				cancels = append(cancels, cancel)
			}
		}
	}
	s.connectionsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	for _, conn := range connections {
		_ = conn.Close()
	}
}

func (s *Server) closeIngressConnections() {
	s.connectionsMu.Lock()
	connections := make([]net.Conn, 0, len(s.connections))
	cancels := make([]context.CancelFunc, 0, len(s.connectionCancels))
	for conn := range s.connections {
		connections = append(connections, conn)
	}
	for _, cancel := range s.connectionCancels {
		cancels = append(cancels, cancel)
	}
	s.connectionsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	for _, conn := range connections {
		_ = conn.Close()
	}
}

func (s *Server) beginSemanticWork() bool {
	s.workMu.Lock()
	defer s.workMu.Unlock()
	if s.shutdownStarted.Load() {
		return false
	}
	s.activeWork++
	select {
	case s.workChanged <- struct{}{}:
	default:
	}
	return true
}

func (s *Server) endSemanticWork() {
	s.workMu.Lock()
	s.activeWork--
	if s.activeWork == 0 {
		s.lastWorkCompletion = time.Now()
	}
	s.workMu.Unlock()
	select {
	case s.workChanged <- struct{}{}:
	default:
	}
	select {
	case s.workIdle <- struct{}{}:
	default:
	}
	s.signalShutdownRetry()
}

func (s *Server) signalShutdownRetry() {
	s.shutdownMu.Lock()
	retry, started := s.shutdownRetry, s.shutdownOwnerStarted
	s.shutdownMu.Unlock()
	if started && retry != nil {
		select {
		case retry <- struct{}{}:
		default:
		}
	}
}

func (s *Server) waitForSemanticWorkFor(timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		s.workMu.Lock()
		active := s.activeWork
		s.workMu.Unlock()
		if active == 0 {
			return true
		}
		if s.workIdle == nil {
			return false
		}
		select {
		case <-s.workIdle:
		case <-timer.C:
			return false
		}
	}
}

func (s *Server) waitForSemanticWork() bool {
	for {
		s.workMu.Lock()
		active := s.activeWork
		s.workMu.Unlock()
		if active == 0 {
			return true
		}
		if s.workIdle == nil {
			return false
		}
		<-s.workIdle
	}
}

func normalizeServerIdleTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 30 * time.Minute
	}
	return timeout
}

func isSemanticDaemonOperation(operation string) bool {
	switch operation {
	case "q", "system.stop", "workspace.add", "workspace.init", "workspace.warm", "workspace.scan", "workspace.doctor", "workspace.hygiene", "workspace.prune", "workspace.remove", "workspace.link", "registry.gc", "index.run", "index.run-job", "index.start", "index.cancel", "worker.install",
		"nav.diagnostics", "nav.symbols", "nav.find", "nav.overview", "nav.outline", "nav.search", "nav.wiki.search", "nav.wiki.validate-harness", "nav.wiki.validate-contract", "nav.wiki.graph", "nav.wiki.validate-source", "nav.wiki.inventory", "nav.wiki.index", "nav.wiki.matrix", "nav.wiki.map", "nav.wiki-root", "nav.evidence.inventory", "nav.governance", "nav.route", "nav.wiki.route", "nav.ask", "nav.pack", "nav.wiki.pack", "nav.service", "nav.refs", "nav.context", "nav.deps", "nav.multi-read", "nav.batch", "nav.related", "nav.neighbors", "nav.callers", "nav.callees", "nav.path", "nav.explain", "nav.graph.stats", "nav.graph.status", "nav.graph.rank", "nav.graph.validate", "nav.graph-impact", "nav.workspace-map", "nav.diff-context", "nav.affected", "nav.flow-slice", "nav.change-pack", "nav.edit-plan", "nav.prepare", "nav.trace", "nav.wiki.trace", "nav.intent", "nav.recall":
		return true
	}
	return false
}

func (s *Server) tryBeginSemanticWork(request model.CommandRequest) bool {
	if request.ProtocolVersion != model.ProtocolVersion || !isSemanticDaemonOperation(request.Operation) {
		return false
	}
	return s.beginSemanticWork()
}

func (s *Server) expireWhenIdle(ctx context.Context, done <-chan struct{}) {
	idleTimeout := normalizeServerIdleTimeout(s.idleTimeout)
	s.workMu.Lock()
	if s.lastWorkCompletion.IsZero() {
		s.lastWorkCompletion = time.Now()
	}
	s.workMu.Unlock()
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()
	jobsWereBusy := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-s.stopped:
			return
		case <-s.workChanged:
			s.workMu.Lock()
			active := s.activeWork
			s.workMu.Unlock()
			if active == 0 {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idleTimeout)
			}
		case <-timer.C:
			s.workMu.Lock()
			active := s.activeWork
			remaining := idleTimeout - time.Since(s.lastWorkCompletion)
			s.workMu.Unlock()
			if active != 0 {
				timer.Reset(idleTimeout)
				continue
			}
			if remaining > 0 {
				timer.Reset(remaining)
				continue
			}
			resumeJobs, jobsIdle := indexer.TryPauseBackgroundJobs()
			if !jobsIdle {
				jobsWereBusy = true
				timer.Reset(idleTimeout)
				continue
			}
			if jobsWereBusy {
				resumeJobs()
				s.workMu.Lock()
				s.lastWorkCompletion = time.Now()
				s.workMu.Unlock()
				jobsWereBusy = false
				timer.Reset(idleTimeout)
				continue
			}
			s.workMu.Lock()
			remaining = idleTimeout - time.Since(s.lastWorkCompletion)
			if s.activeWork == 0 && remaining <= 0 {
				s.shutdownStarted.Store(true)
				s.workMu.Unlock()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownCallerGrace)
				_ = s.shutdownWithFence(shutdownCtx, resumeJobs, false)
				cancel()
				return
			}
			if s.shutdownStarted.Load() {
				s.workMu.Unlock()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownCallerGrace)
				_ = s.shutdownWithFence(shutdownCtx, resumeJobs, false)
				cancel()
				return
			}
			s.workMu.Unlock()
			resumeJobs()
			if remaining > 0 {
				timer.Reset(remaining)
			} else {
				timer.Reset(idleTimeout)
			}
		}
	}
}
