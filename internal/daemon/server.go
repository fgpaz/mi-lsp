package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	elasticdaemon "github.com/fgpaz/bots-herramientas/mold/daemon"
	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
	"github.com/fgpaz/mi-lsp/internal/worker"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type Server struct {
	listener            net.Listener
	app                 *service.App
	manager             *Manager
	telemetry           *TelemetryStore
	admin               *AdminServer
	state               model.DaemonState
	options             StartOptions
	inflight            chan struct{}
	updateGate          *updateAdmissionGate
	updateRetention     *updateRetention
	stopped             chan struct{}
	shutdownMu          sync.Mutex
	shutdownDone        chan struct{}
	shutdownRetry       chan struct{}
	shutdownErr         error
	shutdownFence       func()
	shutdownFenceHasGate bool
	shutdownOwnerStarted bool
	shutdownStarted     atomic.Bool
	elasticLimitsMu     sync.Mutex
	coordinatorSizingMu sync.RWMutex
	coordinatorSource   elasticdaemon.CoordinatorSource
	coordinatorSizing   model.CoordinatorSizingStatus
	resultCache         *resultCache
	qSessions           *service.SessionState
	instanceLease       *startLock
	elastic             *elasticdaemon.Server
	elasticNode         string
	coordinatorCount    int
	elasticCtx          context.Context
	elasticCancel       context.CancelFunc
	elasticReady        chan struct{}
	idleTimeout         time.Duration
	workMu              sync.Mutex
	activeWork          int
	lastWorkCompletion  time.Time
	workChanged         chan struct{}
	workIdle            chan struct{}
	connectionsMu       sync.Mutex
	connections         map[net.Conn]struct{}
	connectionCancels   map[net.Conn]context.CancelFunc
	semanticConnections map[net.Conn]struct{}
	connectionsChanged  chan struct{}
}

// generateAdminToken creates a random 32-byte hex token for admin authentication.
func generateAdminToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func NewServer(repoRoot string, maxWorkers int, idleTimeout time.Duration) (*Server, error) {
	return NewServerWithOptions(repoRoot, maxWorkers, idleTimeout, DefaultStartOptions())
}

func NewServerWithOptions(repoRoot string, maxWorkers int, idleTimeout time.Duration, options StartOptions) (*Server, error) {
	ApplyDaemonMemoryPolicy()
	options = NormalizeStartOptions(options)
	maxWorkers = normalizeMaxWorkers(maxWorkers)
	idleTimeout = normalizeServerIdleTimeout(idleTimeout)
	leasePath, err := daemonInstanceLockPath()
	if err != nil {
		return nil, err
	}
	lease, err := acquireStartLockAt(leasePath, 250*time.Millisecond, time.Now, processExists)
	if err != nil {
		return nil, fmt.Errorf("daemon instance owner lease unavailable: %w", err)
	}
	leaseTransferred := false
	defer func() {
		if !leaseTransferred {
			_ = lease.Close()
		}
	}()
	if err := rejectExistingDaemonOwner(); err != nil {
		return nil, err
	}
	listener, err := listenDaemon()
	if err != nil {
		return nil, err
	}
	telemetry, err := openTelemetryStore()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	listener = newUpdateListener(listener, listenDaemon)
	manager := NewManagerWithOptions(repoRoot, maxWorkers, idleTimeout, options)
	manager.SetTelemetryStore(telemetry)
	elasticCtx, elasticCancel := context.WithCancel(context.Background())
	server := &Server{
		listener:      listener,
		manager:       manager,
		telemetry:     telemetry,
		options:       options,
		inflight:      make(chan struct{}, options.MaxInflight),
		updateGate:    newUpdateAdmissionGate(),
		stopped:       make(chan struct{}),
		shutdownDone:  make(chan struct{}),
		shutdownRetry: make(chan struct{}, 1),
		resultCache:   newResultCache(),
		qSessions:     newQSessionState(),
		instanceLease: lease,
		elasticCtx:    elasticCtx,
		elasticCancel: elasticCancel,
		elasticReady:  make(chan struct{}),
		idleTimeout:   idleTimeout,
		workChanged:   make(chan struct{}, 1),
		workIdle:      make(chan struct{}, 1),
		connections:        make(map[net.Conn]struct{}),
		connectionCancels:  make(map[net.Conn]context.CancelFunc),
		semanticConnections: make(map[net.Conn]struct{}),
		connectionsChanged: make(chan struct{}, 1),
		state: model.DaemonState{
			PID:             os.Getpid(),
			Endpoint:        defaultEndpoint(),
			RepoRoot:        repoRoot,
			StartedAt:       time.Now(),
			Version:         buildVersionString(),
			ProtocolVersion: model.ProtocolVersion,
			MaxWorkers:      maxWorkers,
			IdleTimeout:     idleTimeout.String(),
			WatchMode:       options.WatchMode,
			MaxWatchedRoots: options.MaxWatchedRoots,
			MaxInflight:     options.MaxInflight,
		},
	}
	model.CurrentExecutableSnapshot().ApplyToDaemonState(&server.state)

	// SEC-02: Generate admin token for authentication
	adminToken, err := generateAdminToken()
	if err != nil {
		_ = listener.Close()
		_ = telemetry.Close()
		manager.Shutdown()
		elasticCancel()
		return nil, err
	}
	server.state.AdminToken = adminToken

	server.app = service.New(repoRoot, manager)
	admin, err := newAdminServer(manager, telemetry, server.app, func() model.DaemonState { return server.state }, adminToken, server.updateGate)
	if err != nil {
		_ = listener.Close()
		_ = telemetry.Close()
		manager.Shutdown()
		elasticCancel()
		return nil, err
	}
	server.admin = admin
	server.state.AdminURL = admin.URL()
	if elasticEnabled() {
		if err := server.startElasticPool(); err != nil {
			elasticCancel()
			_ = listener.Close()
			_ = admin.Shutdown()
			manager.Shutdown()
			_ = telemetry.Close()
			return nil, err
		}
	}
	runID, err := telemetry.StartRun(server.state)
	if err != nil {
		elasticCancel()
		_ = listener.Close()
		_ = admin.Shutdown()
		if server.elastic != nil {
			_ = server.elastic.Close()
		}
		manager.Shutdown()
		_ = telemetry.Close()
		return nil, err
	}
	server.state.RunID = runID

	// Start retention ticker to periodically purge old telemetry.
	retDays := daemonRetentionDays()
	maxBytes := int64(50 * 1024 * 1024) // 50MB
	server.updateRetention = newUpdateRetention(server.updateGate, func() { _, _, _ = telemetry.PurgeAndVacuum(retDays, maxBytes) })

	if err := saveDaemonState(server.state); err != nil {
		server.updateRetention.stopAndWait()
		elasticCancel()
		_ = listener.Close()
		_ = admin.Shutdown()
		if server.elastic != nil {
			_ = server.elastic.Close()
		}
		manager.Shutdown()
		_ = telemetry.Close()
		return nil, err
	}
	leaseTransferred = true
	close(server.elasticReady)

	if options.WatchMode == WatchModeEager {
		server.startFileWatchers()
	}
	server.watchElasticLifecycle()
	server.startCoordinatorSizingRefresh()

	return server, nil
}

// startFileWatchers initializes and starts file watchers for all registered workspaces.
func (s *Server) startFileWatchers() {
	registryFile, err := workspace.LoadRegistry()
	if err != nil {
		if os.Getenv("MI_LSP_VERBOSE") != "" {
			fmt.Fprintf(os.Stderr, "[mi-lsp:server] failed to load registry for file watchers: %v\n", err)
		}
		return
	}

	registrations := make([]model.WorkspaceRegistration, 0, len(registryFile.Workspaces))
	for _, reg := range registryFile.Workspaces {
		registrations = append(registrations, reg)
	}

	if len(registrations) > 0 {
		s.manager.StartFileWatchers(registrations)
	}
}

func (s *Server) Serve(ctx context.Context) error {
	return s.serveWithUpdateSchedule(ctx, s.runUpdateSchedule)
}

func (s *Server) serveWithUpdateSchedule(ctx context.Context, schedule func(context.Context)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	idleDone := make(chan struct{})
	go s.expireWhenIdle(ctx, idleDone)
	defer close(idleDone)
	updateCtx, cancelUpdate := context.WithCancel(ctx)
	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		schedule(updateCtx)
	}()
	// Closing ingress during committed shutdown must not let main exit before
	// the controller launches the replacement. Cancel also handles Accept errors.
	defer func() {
		cancelUpdate()
		<-updateDone
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			case <-s.stopped:
				return nil
			default:
			}
			if isRetryableAcceptError(err) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return err
		}
		go s.handleConnectionContext(ctx, conn)
	}
}

// ErrShutdownIncomplete indicates that the caller's wait expired while the
// daemon's single shutdown owner continues draining with resources retained.
var ErrShutdownIncomplete = errors.New("daemon shutdown is still draining")

const (
	shutdownDrainGrace         = 4 * time.Second
	shutdownCallerGrace        = 5 * time.Second
	shutdownFenceRetryInterval = 250 * time.Millisecond
)

// Shutdown keeps the legacy fire-and-wait shape. The owner continues draining
// after this bounded caller wait expires; resources are not released early.
func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownCallerGrace)
	defer cancel()
	_ = s.ShutdownContext(ctx)
}

// ShutdownContext closes admission once and waits only until ctx expires. A
// deadline reports incomplete draining; the single owner retains the DB, lease,
// and any acquired fence until all admitted work and cleanup have completed.
func (s *Server) ShutdownContext(ctx context.Context) error {
	return s.shutdownWithFence(ctx, nil, false)
}

// shutdownWithFence transfers a fence already owned by the update scheduler.
// The caller must not release it after transfer; the shutdown owner releases it
// only after database and lease cleanup.
func (s *Server) shutdownWithFence(ctx context.Context, transferredFence func(), fenceHasGate bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.shutdownMu.Lock()
	if s.shutdownDone == nil {
		s.shutdownDone = make(chan struct{})
	}
	if s.shutdownRetry == nil {
		s.shutdownRetry = make(chan struct{}, 1)
	}
	if transferredFence != nil {
		select {
		case <-s.shutdownDone:
			priorErr := s.shutdownErr
			s.shutdownMu.Unlock()
			transferredFence()
			return priorErr
		default:
		}
		if s.shutdownFence == nil {
			s.shutdownFence = transferredFence
			s.shutdownFenceHasGate = fenceHasGate
		} else {
			// A repeated caller cannot replace or leak a second owned fence.
			s.shutdownMu.Unlock()
			transferredFence()
			s.shutdownMu.Lock()
		}
	}
	s.workMu.Lock()
	s.shutdownStarted.Store(true)
	s.workMu.Unlock()
	if s.stopped != nil {
		select {
		case <-s.stopped:
		default:
			close(s.stopped)
		}
	}
	if !s.shutdownOwnerStarted {
		s.shutdownOwnerStarted = true
		go s.runShutdownOwner()
	}
	select {
	case s.shutdownRetry <- struct{}{}:
	default:
	}
	done := s.shutdownDone
	s.shutdownMu.Unlock()
	select {
	case <-done:
		s.shutdownMu.Lock()
		err := s.shutdownErr
		s.shutdownMu.Unlock()
		return err
	default:
	}
	select {
	case <-done:
		s.shutdownMu.Lock()
		err := s.shutdownErr
		s.shutdownMu.Unlock()
		return err
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrShutdownIncomplete, ctx.Err())
	}
}

func (s *Server) runShutdownOwner() {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.closeIdleIngressConnections()
	var cleanupErr error
	if s.admin != nil {
		cleanupErr = errors.Join(cleanupErr, s.admin.Shutdown())
	}
	_ = s.waitForSemanticWorkFor(shutdownDrainGrace)
	// Drain expiry or natural semantic completion both close residual handlers.
	s.closeIngressConnections()
	for !s.waitForSemanticWork() || !s.waitForIngressHandlers() {
		<-s.shutdownRetry
	}
	if s.updateRetention != nil {
		s.updateRetention.stopAndWait()
	}

	retryTicker := time.NewTicker(shutdownFenceRetryInterval)
	defer retryTicker.Stop()
	fence, fenceHasGate := s.takeShutdownFence()
	for fence == nil || (s.updateGate != nil && !fenceHasGate) {
		var releaseGate func()
		gateReady := fenceHasGate || s.updateGate == nil
		if !gateReady && s.updateGate != nil {
			releaseGate, gateReady = s.updateGate.TryIdleBarrier()
		}
		if !gateReady && fence != nil {
			if !s.restoreShutdownFence(fence) {
				fence()
			}
			fence = nil
		}
		if gateReady {
			resumeJobs := fence
			jobsIdle := resumeJobs != nil
			if !jobsIdle {
				resumeJobs, jobsIdle = indexer.TryPauseBackgroundJobs()
			}
			if jobsIdle {
				if fenceHasGate {
					break
				}
				if releaseGate == nil {
					fence = resumeJobs
				} else {
					fence = func() { releaseGate(); resumeJobs() }
				}
				fenceHasGate = true
				break
			}
			if releaseGate != nil {
				releaseGate()
			}
		}
		select {
		case <-s.shutdownRetry:
		case <-retryTicker.C:
		}
		fence, fenceHasGate = s.takeShutdownFence()
	}

	if s.elasticCancel != nil {
		s.elasticCancel()
	}
	if s.manager != nil {
		s.syncRuntimeSnapshots()
	}
	if s.elastic != nil {
		s.elasticLimitsMu.Lock()
		cleanupErr = errors.Join(cleanupErr, s.elastic.Close())
		s.elasticLimitsMu.Unlock()
	}
	if s.manager != nil {
		s.manager.Shutdown()
	}
	cleanupErr = errors.Join(cleanupErr, clearDaemonState())
	if s.telemetry != nil {
		cleanupErr = errors.Join(cleanupErr, s.telemetry.StopRun(s.state.RunID, time.Now()))
	}
	if s.telemetry != nil {
		if err := s.telemetry.Close(); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
			s.finishShutdown(cleanupErr)
			return
		}
	}
	if s.instanceLease != nil {
		if err := s.instanceLease.Close(); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
			s.finishShutdown(cleanupErr)
			return
		}
	}
	if fence != nil {
		fence()
	}
	s.finishShutdown(cleanupErr)
}

func (s *Server) restoreShutdownFence(fence func()) bool {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	if s.shutdownFence != nil {
		return false
	}
	s.shutdownFence = fence
	s.shutdownFenceHasGate = false
	return true
}

func (s *Server) finishShutdown(err error) {
	s.shutdownMu.Lock()
	s.shutdownErr = err
	close(s.shutdownDone)
	s.shutdownMu.Unlock()
}

func (s *Server) takeShutdownFence() (func(), bool) {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	fence, hasGate := s.shutdownFence, s.shutdownFenceHasGate
	s.shutdownFence = nil
	s.shutdownFenceHasGate = false
	return fence, hasGate
}

func (s *Server) handleConnection(conn net.Conn) {
	s.handleConnectionContext(context.Background(), conn)
}

func (s *Server) handleConnectionContext(ctx context.Context, conn net.Conn) {
	if !s.registerIngressConnection(conn) {
		_ = conn.Close()
		return
	}
	defer s.unregisterIngressConnection(conn)
	if s.updateGate != nil {
		finish, ok := s.updateGate.Begin("accepted-connection")
		if !ok {
			_ = conn.Close()
			return
		}
		defer finish()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer conn.Close()

	var request model.CommandRequest
	if err := worker.ReadFrame(conn, &request); err != nil {
		_ = worker.WriteFrame(conn, daemonErrorEnvelope(request, err, "transport"))
		return
	}
	if s.shutdownStarted.Load() {
		_ = conn.Close()
		return
	}
	started := time.Now()
	if s.isBackpressureLimited(request) && !s.tryAcquireInflight() {
		response := s.backpressureEnvelope(request)
		s.recordAccess(request, response, nil, time.Since(started))
		_ = worker.WriteFrame(conn, response)
		return
	}
	if s.isBackpressureLimited(request) {
		defer s.releaseInflight()
	}
	// Keep the actual service/root context so its deadlines and cancellation
	// propagate to the handler; peer EOF cancellation remains request-local.
	requestCtx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()
	semanticEligible := request.ProtocolVersion == model.ProtocolVersion && isSemanticDaemonOperation(request.Operation)
	if !s.trackConnectionHandler(conn, cancelRequest, semanticEligible) {
		_ = conn.Close()
		return
	}
	defer s.detachConnectionCancel(conn)
	if semanticEligible {
		defer s.endSemanticWork()
	}
	if request.Operation == "q" {
		go func() {
			var probe [1]byte
			if _, readErr := conn.Read(probe[:]); readErr != nil {
				cancelRequest()
			}
		}()
	}
	var response model.Envelope
	if s.elastic != nil {
		response = s.submitElastic(requestCtx, request)
	} else {
		var err error
		response, err = s.handleRequestContext(requestCtx, request)
		if err != nil {
			response = daemonErrorEnvelope(request, err, "daemon")
		}
		s.recordAccess(request, response, err, time.Since(started))
	}
	_ = worker.WriteFrame(conn, response)
}

func (s *Server) handleRequest(request model.CommandRequest) (model.Envelope, error) {
	return s.handleRequestContext(context.Background(), request)
}

func (s *Server) handleRequestContext(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if request.ProtocolVersion == "" || request.ProtocolVersion != model.ProtocolVersion {
		return model.Envelope{}, fmt.Errorf("protocol version mismatch: client=%s daemon=%s", request.ProtocolVersion, model.ProtocolVersion)
	}

	switch request.Operation {
	case "q":
		return s.app.ExecuteQ(ctx, request, s.qSessions)
	case "system.status":
		// Default to 5 recent accesses for token efficiency.
		// Opt-in to 20 with --telemetry flag or full context.
		// recent_accesses defaults to a compact view (5 events, path fields stripped) for
		// token efficiency; pass --verbose (or --full) to get the full 20-event detail.
		verboseTelemetry := request.Context.Full || request.Context.Verbose
		recentAccessCount := 5
		if verboseTelemetry {
			recentAccessCount = 20
		}
		var accesses []model.AccessEvent
		if s.telemetry != nil {
			accesses, _ = s.telemetry.RecentAccesses(recentAccessCount)
		}
		if !verboseTelemetry {
			// TOK-03: strip the long, repeated absolute-path fields from the compact
			// recent_accesses view to cut token cost (workspace_root and runtime_key embed
			// the full root on every event). The workspace alias is retained.
			for i := range accesses {
				accesses[i].WorkspaceRoot = ""
				accesses[i].RuntimeKey = ""
				accesses[i].EntrypointID = ""
			}
		}
		hits, misses, entries := s.resultCache.stats()
		total := hits + misses
		hitRate := 0.0
		if total > 0 {
			hitRate = float64(hits) / float64(total)
		}
		var poolStats any
		if s.elastic != nil {
			poolStats = s.elastic.Stats()
		}
		return model.Envelope{
			Ok:      true,
			Backend: "daemon",
			Items: []map[string]any{{
				"state":           s.state,
				"daemon_process":  processStats(os.Getpid()),
				"watchers":        s.manager.WatcherStats(),
				"active_runtimes": s.manager.Status(),
				"recent_accesses": accesses,
				"telemetry_wal":   telemetryWALStatus(s.telemetry),
				"result_cache": map[string]any{
					"hits":     hits,
					"misses":   misses,
					"entries":  entries,
					"hit_rate": hitRate,
				},
				"elastic_pool":               poolStats,
				"coordinator_node":           s.elasticNode,
				"coordinator_count_at_start": s.coordinatorCount,
				"coordinator_sizing":         s.coordinatorSizingSnapshot(),
			}},
		}, nil
	case "system.stop":
		go s.Shutdown()
		return model.Envelope{Ok: true, Backend: "daemon", Items: []string{"stopping daemon"}}, nil
	case "worker.status":
		return s.app.Execute(ctx, request)
	case "workspace.add", "workspace.init":
		// For workspace add/init, start async background indexing instead of blocking on sync index.
		// Extract workspace root from payload and queue async job.
		if path, ok := request.Payload["path"].(string); ok && path != "" {
			// Normalize path and start background index
			jobID := startWorkspaceBackgroundIndex(ctx, path)
			// Store job ID in context for later retrieval if needed
			if jobID != "" {
				request.Payload["_index_job_id"] = jobID
			}
		}
		// Proceed with normal registration flow
		return s.app.Execute(ctx, request)
	case "workspace.warm":
		resolution, err := workspace.ResolveWorkspaceSelection(request.Context.Workspace, request.Context.CallerCWD)
		if err != nil {
			return model.Envelope{}, err
		}
		registration := resolution.Registration
		warnings := append([]string{}, resolution.Warnings...)
		warnings = append(warnings, s.manager.Warm(registration)...)
		// Pre-warm graph join caches (rank/stats) so first harness packet is hot.
		if warmWarnings := s.warmGraphJoinCaches(ctx, registration, request.Context); len(warmWarnings) > 0 {
			warnings = append(warnings, warmWarnings...)
		}
		statuses := make([]model.WorkerStatus, 0)
		for _, status := range s.manager.Status() {
			if status.Workspace == registration.Name {
				statuses = append(statuses, status)
			}
		}
		items := []string{"workspace warmed"}
		if len(statuses) > 0 {
			items = []string{"workspace warmed", statusSummary(statuses)}
		}
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "daemon", Items: items, Warnings: warnings}, nil
	default:
		// Attempt result caching for deterministic read-only operations
		if s.resultCache != nil && isCacheableOp(request.Operation) {
			resolution, resolveErr := workspace.ResolveWorkspaceSelection(request.Context.Workspace, request.Context.CallerCWD)
			if resolveErr == nil {
				canonicalRoot, generation, identityErr := indexGeneration(resolution.Registration.Root)
				// Extract canonical args from request (use query, limit, offset, etc.)
				canonicalArgs := extractCanonicalArgs(request)
				cacheKey, keyErr := resultCacheKey(canonicalRoot, request.Operation, generation, canonicalArgs)
				if identityErr != nil {
					keyErr = identityErr
				}
				if keyErr == nil {
					// Try cache hit
					if cachedBytes, ok := s.resultCache.get(cacheKey); ok {
						var cachedEnvelope model.Envelope
						if err := json.Unmarshal(cachedBytes, &cachedEnvelope); err == nil {
							return cachedEnvelope, nil
						}
					} else {
						// Cache miss: log debug info if enabled
						if os.Getenv("MI_LSP_DAEMON_RESULT_CACHE_DEBUG") != "" {
							argsJSON, _ := json.Marshal(canonicalArgs)
							argsStr := string(argsJSON)
							if len(argsStr) > 200 {
								argsStr = argsStr[:200] + "..."
							}
							fmt.Fprintf(os.Stderr, "[cache:debug:miss] op=%s key=%s args_sample=%s\n",
								request.Operation, cacheKey[:minInt(12, len(cacheKey))], argsStr)
						}
					}
				}
			}
		}

		// Execute operation.
		response, err := s.app.Execute(ctx, request)
		// Graph queries are executed by the canonical service contract. Preserve its
		// payload exactly, while identifying the daemon-routed SQLite result.
		if err == nil && response.Ok && isGraphQueryOperation(request.Operation) {
			response.Backend = "daemon/sqlite"
		}

		// Cache successful results for cacheable ops
		if err == nil && response.Ok && s.resultCache != nil && isCacheableOp(request.Operation) {
			resolution, resolveErr := workspace.ResolveWorkspaceSelection(request.Context.Workspace, request.Context.CallerCWD)
			if resolveErr == nil {
				canonicalRoot, generation, identityErr := indexGeneration(resolution.Registration.Root)
				canonicalArgs := extractCanonicalArgs(request)
				cacheKey, keyErr := resultCacheKey(canonicalRoot, request.Operation, generation, canonicalArgs)
				if identityErr != nil {
					keyErr = identityErr
				}
				if keyErr == nil {
					if envelopeBytes, marshalErr := json.Marshal(response); marshalErr == nil {
						s.resultCache.set(cacheKey, envelopeBytes, resultCacheTTL)
					}
				}
			}
		}

		return response, err
	}
}

func (s *Server) recordAccess(request model.CommandRequest, response model.Envelope, operationErr error, latency time.Duration) {
	if s.telemetry == nil {
		return
	}
	seq := s.telemetry.NextSeq(request.Context.SessionID)
	event := model.AccessEvent{
		OccurredAt:     time.Now(),
		ClientName:     firstNonEmpty(request.Context.ClientName, "manual-cli"),
		SessionID:      request.Context.SessionID,
		Seq:            seq,
		Repo:           payloadString(request.Payload, "repo"),
		Operation:      request.Operation,
		Intent:         requestTelemetryIntent(request, response),
		Backend:        response.Backend,
		Route:          "daemon",
		Format:         request.Context.Format,
		TokenBudget:    request.Context.TokenBudget,
		MaxItems:       request.Context.MaxItems,
		MaxChars:       request.Context.MaxChars,
		Compress:       request.Context.Compress,
		Success:        operationErr == nil && response.Ok,
		LatencyMs:      latency.Milliseconds(),
		Warnings:       response.Warnings,
		WorkspaceInput: request.Context.Workspace,
		Workspace:      firstNonEmpty(response.Workspace, request.Context.Workspace),
		RuntimeKey:     telemetry.RuntimeKeyForOperation(request, response),
		EntrypointID:   firstNonEmpty(payloadString(request.Payload, "entrypoint"), payloadString(request.Payload, "solution"), payloadString(request.Payload, "project_path")),
		BytesOut:       telemetry.EnvelopeBytes(response),
		Harness:        telemetry.SanitizeHarness(request.Context.Harness),
	}
	if operationErr != nil {
		event.Error = operationErr.Error()
	}
	if isBackpressureEnvelope(response) {
		event.ErrorKind = "daemon"
		event.ErrorCode = "backpressure_busy"
		event.Error = firstNonEmpty(event.Error, "daemon/backpressure_busy")
	}
	event = telemetry.EnrichAccessEvent(event, request, response, operationErr)
	_ = s.telemetry.RecordAccess(s.state.RunID, event)
	_ = s.telemetry.ReplaceRuntimeSnapshots(s.state.RunID, s.manager.Status())
}

func (s *Server) syncRuntimeSnapshots() {
	if s.telemetry == nil {
		return
	}
	_ = s.telemetry.ReplaceRuntimeSnapshots(s.state.RunID, s.manager.Status())
}

func SpawnBackground(repoRoot string, maxWorkers int, idleTimeout time.Duration) (model.DaemonState, bool, error) {
	return SpawnBackgroundWithOptions(repoRoot, maxWorkers, idleTimeout, DefaultStartOptions())
}

func SpawnBackgroundWithOptions(repoRoot string, maxWorkers int, idleTimeout time.Duration, options StartOptions) (model.DaemonState, bool, error) {
	if elasticEnabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := readBabelNode(ctx)
		cancel()
		if err != nil {
			return model.DaemonState{}, false, err
		}
	}
	options = NormalizeStartOptions(options)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if state, err := probeDaemon(ctx); err == nil {
		state.AlreadyRunning = true
		return state, false, nil
	}

	lock, err := acquireStartLock(10 * time.Second)
	if err != nil {
		return model.DaemonState{}, false, err
	}
	defer lock.Close()

	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if state, err := probeDaemon(ctx); err == nil {
		state.AlreadyRunning = true
		return state, false, nil
	}
	if err := rejectExistingDaemonOwner(); err != nil {
		return model.DaemonState{}, false, err
	}

	command, err := daemonServeCommand(repoRoot, maxWorkers, idleTimeout, options)
	if err != nil {
		return model.DaemonState{}, false, err
	}
	if err := command.Start(); err != nil {
		return model.DaemonState{}, false, err
	}
	releaseStartedCommand(command)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		state, probeErr := probeDaemon(ctx)
		cancel()
		if probeErr == nil {
			return state, true, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	state, loadErr := loadDaemonState()
	if loadErr == nil {
		return state, true, fmt.Errorf("daemon start timed out before health check succeeded; stale state pid=%d endpoint=%s", state.PID, state.Endpoint)
	}
	return model.DaemonState{}, false, errors.New("daemon start timed out before health check succeeded")
}

func (s *Server) tryAcquireInflight() bool {
	select {
	case s.inflight <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) releaseInflight() {
	select {
	case <-s.inflight:
	default:
	}
}

func (s *Server) warmGraphJoinCaches(ctx context.Context, registration model.WorkspaceRegistration, opts model.QueryOptions) []string {
	if s.app == nil {
		return nil
	}
	warnings := []string{}
	// Seed result cache with graph rank + stats for the active generation.
	for _, op := range []string{"nav.graph.rank", "nav.graph.stats"} {
		req := model.CommandRequest{
			ProtocolVersion: model.ProtocolVersion,
			Operation:       op,
			Context:         opts,
			Payload:         map[string]any{"limit": 32},
		}
		req.Context.Workspace = registration.Name
		env, err := s.app.Execute(ctx, req)
		if err != nil {
			warnings = append(warnings, op+" warm skipped: "+err.Error())
			continue
		}
		if !env.Ok {
			continue
		}
		if s.resultCache == nil || !isCacheableOp(op) {
			continue
		}
		canonicalRoot, generation, identityErr := indexGeneration(registration.Root)
		if identityErr != nil {
			continue
		}
		canonicalArgs := extractCanonicalArgs(req)
		cacheKey, keyErr := resultCacheKey(canonicalRoot, op, generation, canonicalArgs)
		if keyErr != nil {
			continue
		}
		if envelopeBytes, marshalErr := json.Marshal(env); marshalErr == nil {
			s.resultCache.set(cacheKey, envelopeBytes, resultCacheTTL)
		}
	}
	return warnings
}

func isGraphQueryOperation(operation string) bool {
	switch operation {
	case "nav.neighbors", "nav.callers", "nav.callees", "nav.path", "nav.explain", "nav.graph.stats", "nav.graph.status", "nav.graph.rank", "nav.graph.validate":
		return true
	default:
		return false
	}
}

func (s *Server) isBackpressureLimited(request model.CommandRequest) bool {
	switch request.Operation {
	case "q", "nav.refs", "nav.context", "nav.deps", "nav.related", "nav.service", "nav.diff-context", "nav.batch", "nav.search", "nav.find", "nav.neighbors", "nav.callers", "nav.callees", "nav.path", "nav.explain", "nav.graph.stats", "nav.graph.status", "nav.graph.rank", "nav.graph.validate", "nav.intent", "nav.flow-slice", "nav.change-pack", "nav.diagnostics", "workspace.warm":
		return true
	case "nav.workspace-map":
		return request.Context.Full
	default:
		return false
	}
}

func (s *Server) backpressureEnvelope(request model.CommandRequest) model.Envelope {
	message := fmt.Sprintf("daemon/backpressure_busy: %s exceeds max inflight %d", request.Operation, s.options.MaxInflight)
	hint := "retry later or raise MI_LSP_DAEMON_MAX_INFLIGHT for this machine"
	return model.Envelope{
		Ok:      false,
		Backend: "daemon",
		Items: []map[string]any{{
			"error_kind":   "daemon",
			"error_code":   "backpressure_busy",
			"operation":    request.Operation,
			"max_inflight": s.options.MaxInflight,
		}},
		Warnings: []string{message},
		Hint:     hint,
		Error: &model.EnvelopeError{
			Kind:      "daemon",
			Code:      "backpressure_busy",
			Message:   message,
			Stage:     "backend",
			HintCode:  "backpressure_busy",
			Retryable: true,
		},
	}
}

func daemonErrorEnvelope(request model.CommandRequest, err error, fallbackKind string) model.Envelope {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "daemon request failed"
	}
	backend := "daemon"
	if request.Operation != "" && !strings.EqualFold(strings.TrimSpace(fallbackKind), "transport") {
		switch {
		case strings.HasPrefix(request.Operation, "workspace."):
			backend = "workspace"
		case strings.HasPrefix(request.Operation, "index."):
			backend = "index"
		case strings.HasPrefix(request.Operation, "worker."):
			backend = "worker"
		case strings.HasPrefix(request.Operation, "nav.wiki."):
			backend = "wiki"
		case strings.HasPrefix(request.Operation, "nav."):
			backend = "nav"
		}
	}
	envErr := model.EnvelopeError{Kind: "backend_runtime", Code: "daemon_request_failed", Message: message, Stage: "backend"}
	if strings.EqualFold(strings.TrimSpace(fallbackKind), "transport") {
		envErr.Kind = "transport"
		envErr.Code = "daemon_transport_failed"
		envErr.Stage = "transport"
		envErr.HintCode = "daemon_transport_failed"
		envErr.Retryable = true
	} else if observationErr := (*model.GraphObservationError)(nil); errors.As(err, &observationErr) {
		code := strings.ToLower(strings.TrimSpace(observationErr.Code))
		if code == "" {
			code = "graph_observation_failed"
		}
		envErr.Kind = "backend_runtime"
		envErr.Code = code
		envErr.Message = observationErr.Message
		envErr.Stage = "graph_observation"
		envErr.HintCode = code
		envErr.ReasonCode = "explicit_incomplete"
		envErr.Detail = "field=" + strings.TrimSpace(observationErr.Field) + "; " + observationErr.Message
	} else if info := telemetry.ClassifyErrorInfo(backend, message, nil); strings.TrimSpace(info.Kind) != "" || strings.TrimSpace(info.Code) != "" {
		if strings.TrimSpace(info.Kind) != "" {
			envErr.Kind = info.Kind
		}
		if code := strings.TrimSpace(info.Code); code != "" && code != "_generic" {
			if backend == "index" && code == "index_generic" {
				envErr.Code = "index_failed"
				envErr.HintCode = "index_failed"
				envErr.Detail = message
			} else {
				envErr.Code = code
			}
		}
	}
	env := model.Envelope{
		Ok:        false,
		Workspace: request.Context.Workspace,
		Backend:   backend,
		Items:     []map[string]any{},
		Warnings:  []string{envErr.Kind + "/" + envErr.Code},
		Error:     &envErr,
	}
	if continuation, detail, ok := workspace.ContinuationForUnresolvedSelector(request.Operation, err); ok {
		env.Continuation = continuation
		env.Error.Kind = "workspace"
		env.Error.Code = "workspace_resolution_failed"
		env.Error.Stage = "selector_validation"
		env.Error.HintCode = "workspace_resolution_failed"
		env.Error.ReasonCode = "invalid_workspace"
		env.Error.Detail = detail
		env.Warnings = []string{"workspace/workspace_resolution_failed"}
	}
	return env
}

func probeDaemon(ctx context.Context) (model.DaemonState, error) {
	response, err := NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: model.QueryOptions{ClientName: "mi-lsp-daemon-probe"}})
	if err != nil {
		return model.DaemonState{}, err
	}
	if len(response.Warnings) > 0 && !response.Ok {
		return model.DaemonState{}, errors.New(strings.Join(response.Warnings, "; "))
	}
	body, marshalErr := json.Marshal(response.Items)
	if marshalErr != nil {
		return model.DaemonState{}, marshalErr
	}
	var decoded []map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded) == 0 {
		return loadDaemonState()
	}
	stateBody, marshalErr := json.Marshal(decoded[0]["state"])
	if marshalErr != nil {
		return model.DaemonState{}, marshalErr
	}
	var state model.DaemonState
	if err := json.Unmarshal(stateBody, &state); err != nil {
		return model.DaemonState{}, err
	}
	return state, nil
}

func startWorkspaceBackgroundIndex(ctx context.Context, path string) string {
	indexCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	jobID, _ := indexer.StartBackgroundIndex(indexCtx, path, false, indexer.IndexModeFull)
	return jobID
}

func requestTelemetryIntent(request model.CommandRequest, response model.Envelope) string {
	return telemetry.IntentFromRequestEnvelope(request, response)
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isBackpressureEnvelope(response model.Envelope) bool {
	if response.Ok {
		return false
	}
	for _, warning := range response.Warnings {
		if strings.Contains(warning, "daemon/backpressure_busy") {
			return true
		}
	}
	return false
}

func isRetryableAcceptError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "use of closed network connection") || strings.Contains(message, "pipe has been ended") {
		return true
	}
	return errors.Is(err, net.ErrClosed)
}

func BuildStatusError() error {
	return fmt.Errorf("daemon is not running")
}

func daemonRetentionDays() int {
	raw := os.Getenv("MI_LSP_RETENTION_DAYS")
	if raw == "" {
		return 30
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 {
		return 30
	}
	return days
}

// buildVersionString uses the release version injected for the CLI, falling
// back to Go build info for local builds.
func buildVersionString() string {
	version := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		version = info.Main.Version
	}
	version, _ = model.ReleaseMetadata(version, "")
	return version
}

// extractCanonicalArgs extracts ALL semantically relevant arguments from a request for cache keying.
// Serializes the complete payload + envelope flags (Format, AXI, Full, Verbose, Compress, etc.)
// with deterministic JSON ordering, excluding only non-semantic fields:
// session_id, client_name, workspace (already in cache key), caller_cwd, workspace_source, token_budget, backend_hint, allow_cross_workspace.
func extractCanonicalArgs(request model.CommandRequest) map[string]any {
	result := make(map[string]any)

	// Copy ALL payload fields (payload affects result semantically)
	for k, v := range request.Payload {
		result[k] = v
	}

	// Include semantic envelope flags from Context
	// These flags alter the result format and content:
	if request.Context.Format != "" {
		result["_format"] = request.Context.Format
	}
	if request.Context.MaxItems > 0 {
		result["_max_items"] = request.Context.MaxItems
	}
	if request.Context.MaxChars > 0 {
		result["_max_chars"] = request.Context.MaxChars
	}
	if request.Context.AXI {
		result["_axi"] = true
	}
	if request.Context.Full {
		result["_full"] = true
	}
	if request.Context.Verbose {
		result["_verbose"] = true
	}
	if request.Context.Compress {
		result["_compress"] = true
	}
	if request.Context.Offset > 0 {
		result["_offset"] = request.Context.Offset
	}
	if request.Operation == "nav.prepare" {
		if task := payloadString(request.Payload, "task"); task != "" {
			result["_task_digest"] = service.PreparationDigest(task)
		}
		if resolution, err := workspace.ResolveWorkspaceSelection(request.Context.Workspace, request.Context.CallerCWD); err == nil {
			if canonicalRoot, _, identityErr := service.PreparationCacheIdentity(resolution.Registration.Root); identityErr == nil {
				result["_governance_digest"] = service.PreparationGovernanceDigest(canonicalRoot)
			}
		}
		if plan := firstPayloadString(request.Payload, "plan", "packet", "edit_plan"); plan != "" {
			result["_plan_digest"] = service.PreparationDigest(plan)
		}
	}

	return result
}

func firstPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
