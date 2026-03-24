package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/worker"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type Server struct {
	listener  net.Listener
	app       *service.App
	manager   *Manager
	telemetry *TelemetryStore
	admin     *AdminServer
	state     model.DaemonState
	stopped   chan struct{}
	stopOnce  sync.Once
}

func NewServer(repoRoot string, maxWorkers int, idleTimeout time.Duration) (*Server, error) {
	listener, err := listenDaemon()
	if err != nil {
		return nil, err
	}
	telemetry, err := openTelemetryStore()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	manager := NewManager(repoRoot, maxWorkers, idleTimeout)
	server := &Server{
		listener:  listener,
		manager:   manager,
		telemetry: telemetry,
		stopped:   make(chan struct{}),
		state: model.DaemonState{
			PID:             os.Getpid(),
			Endpoint:        defaultEndpoint(),
			RepoRoot:        repoRoot,
			StartedAt:       time.Now(),
			Version:         "dev",
			ProtocolVersion: model.ProtocolVersion,
			MaxWorkers:      maxWorkers,
			IdleTimeout:     idleTimeout.String(),
		},
	}
	server.app = service.New(repoRoot, manager)
	admin, err := NewAdminServer(manager, telemetry, server.app, func() model.DaemonState { return server.state })
	if err != nil {
		_ = listener.Close()
		_ = telemetry.Close()
		return nil, err
	}
	server.admin = admin
	server.state.AdminURL = admin.URL()
	runID, err := telemetry.StartRun(server.state)
	if err != nil {
		_ = listener.Close()
		_ = admin.Shutdown()
		_ = telemetry.Close()
		return nil, err
	}
	server.state.RunID = runID

	// Apply retention on daemon startup.
	retDays := daemonRetentionDays()
	cutoff := time.Now().AddDate(0, 0, -retDays)
	_, _ = telemetry.PurgeOldEvents(cutoff)
	_, _ = telemetry.PurgeOldRuns(cutoff)

	if err := saveDaemonState(server.state); err != nil {
		_ = listener.Close()
		_ = admin.Shutdown()
		_ = telemetry.Close()
		return nil, err
	}

	// Start file watchers for registered workspaces
	server.startFileWatchers()

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
		go s.handleConnection(conn)
	}
}

func (s *Server) Shutdown() {
	s.stopOnce.Do(func() {
		close(s.stopped)
		s.syncRuntimeSnapshots()
		_ = clearDaemonState()
		_ = s.telemetry.StopRun(s.state.RunID, time.Now())
		_ = s.listener.Close()
		_ = s.admin.Shutdown()
		s.manager.Shutdown()
		_ = s.telemetry.Close()
	})
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	var request model.CommandRequest
	if err := worker.ReadFrame(conn, &request); err != nil {
		_ = worker.WriteFrame(conn, model.Envelope{Ok: false, Backend: "daemon", Items: []string{}, Warnings: []string{err.Error()}})
		return
	}
	started := time.Now()
	response, err := s.handleRequest(request)
	if err != nil {
		response = model.Envelope{Ok: false, Backend: "daemon", Items: []string{}, Warnings: []string{err.Error()}}
	}
	s.recordAccess(request, response, err, time.Since(started))
	_ = worker.WriteFrame(conn, response)
}

func (s *Server) handleRequest(request model.CommandRequest) (model.Envelope, error) {
	if request.ProtocolVersion != "" && request.ProtocolVersion != model.ProtocolVersion {
		return model.Envelope{}, fmt.Errorf("protocol version mismatch: client=%s daemon=%s", request.ProtocolVersion, model.ProtocolVersion)
	}

	switch request.Operation {
	case "system.status":
		accesses, _ := s.telemetry.RecentAccesses(20)
		return model.Envelope{
			Ok:      true,
			Backend: "daemon",
			Items: []map[string]any{{
				"state":           s.state,
				"active_runtimes": s.manager.Status(),
				"recent_accesses": accesses,
			}},
		}, nil
	case "system.stop":
		go s.Shutdown()
		return model.Envelope{Ok: true, Backend: "daemon", Items: []string{"stopping daemon"}}, nil
	case "worker.status":
		return model.Envelope{Ok: true, Backend: "daemon", Items: s.manager.Status()}, nil
	case "workspace.warm":
		registration, err := s.app.ResolveWorkspace(request.Context.Workspace)
		if err != nil {
			return model.Envelope{}, err
		}
		warnings := s.manager.Warm(registration)
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
		response, err := s.app.Execute(context.Background(), request)
		return response, err
	}
}

func (s *Server) recordAccess(request model.CommandRequest, response model.Envelope, operationErr error, latency time.Duration) {
	if s.telemetry == nil {
		return
	}
	event := model.AccessEvent{
		OccurredAt:   time.Now(),
		ClientName:   firstNonEmpty(request.Context.ClientName, "manual-cli"),
		SessionID:    request.Context.SessionID,
		Workspace:    request.Context.Workspace,
		Repo:         payloadString(request.Payload, "repo"),
		Operation:    request.Operation,
		Backend:      response.Backend,
		Success:      operationErr == nil && response.Ok,
		LatencyMs:    latency.Milliseconds(),
		Warnings:     response.Warnings,
		RuntimeKey:   runtimeKeyFromEnvelope(request, response),
		EntrypointID: firstNonEmpty(payloadString(request.Payload, "entrypoint"), payloadString(request.Payload, "solution"), payloadString(request.Payload, "project_path")),
	}
	if operationErr != nil {
		event.Error = operationErr.Error()
	}
	count := 0
	if rv := reflect.ValueOf(response.Items); rv.IsValid() && rv.Kind() == reflect.Slice {
		count = rv.Len()
	}
	event.ResultCount = count
	maxItems := request.Context.MaxItems
	if maxItems <= 0 {
		maxItems = 50
	}
	event.Truncated = count > 0 && count >= maxItems
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

	command, err := daemonServeCommand(repoRoot, maxWorkers, idleTimeout)
	if err != nil {
		return model.DaemonState{}, false, err
	}
	if err := command.Start(); err != nil {
		return model.DaemonState{}, false, err
	}

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
		return state, true, nil
	}
	return model.DaemonState{}, false, errors.New("daemon start timed out before health check succeeded")
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

func runtimeKeyFromEnvelope(request model.CommandRequest, response model.Envelope) string {
	backendType := firstNonEmpty(response.Backend, request.Context.BackendHint, payloadString(request.Payload, "backend_type"))
	if backendType == "" {
		backendType = "catalog"
	}
	workspaceName := firstNonEmpty(request.Context.Workspace, response.Workspace, "-")
	entrypoint := firstNonEmpty(payloadString(request.Payload, "entrypoint"), payloadString(request.Payload, "solution"), payloadString(request.Payload, "project_path"), payloadString(request.Payload, "repo"), "default")
	return backendType + "::" + workspaceName + "::" + entrypoint
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
