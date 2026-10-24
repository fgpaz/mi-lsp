package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	elasticdaemon "github.com/fgpaz/bots-herramientas/mold/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
)

const elasticCommandOperation = "mi-lsp.command"

func (s *Server) startElasticPool() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	node, err := readBabelNode(ctx)
	if err != nil {
		return err
	}
	source := elasticdaemon.DefaultCoordinatorSource(node)
	sizing, err := elasticdaemon.CoordinatorSizing(ctx, node, source, 0)
	if err != nil {
		return fmt.Errorf("read live coordinator census for node %q: %w", node, err)
	}

	pool, err := elasticdaemon.Start(elasticdaemon.Config{
		Name:       "mi-lsp-" + node,
		MinWorkers: sizing.Minimum,
		MaxWorkers: sizing.Maximum,
		// The existing mi-lsp daemon process owns idle shutdown and its configured
		// timeout. The common server must not close its Submit queue independently.
		AlwaysOn: true,
		Handler:  s.handleElasticOperation,
	})
	if err != nil {
		return fmt.Errorf("start shared daemon pool for node %q: %w", node, err)
	}
	s.elastic = pool
	s.elasticNode = node
	s.coordinatorCount = sizing.Count
	return nil
}

func readBabelNode(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "botsd", "identidad").Output()
	if err != nil {
		return "", fmt.Errorf("read public Babel node identity with botsd identidad: %w", err)
	}
	var identity struct {
		Node string `json:"nodo"`
	}
	if err := json.Unmarshal(output, &identity); err != nil {
		return "", fmt.Errorf("decode public Babel node identity: %w", err)
	}
	node := strings.TrimSpace(identity.Node)
	if node == "" {
		return "", errors.New("botsd identidad returned no node identity")
	}
	for _, r := range node {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return "", errors.New("botsd identidad returned a node name invalid for the shared daemon endpoint")
		}
	}
	return node, nil
}

func (s *Server) handleElasticOperation(ctx context.Context, request elasticdaemon.Request) elasticdaemon.Response {
	select {
	case <-s.elasticReady:
	case <-s.elasticCtx.Done():
		return elasticdaemon.Response{OK: false, ReasonCode: "daemon_stopping", Detail: "mi-lsp daemon is shutting down"}
	case <-ctx.Done():
		return elasticdaemon.Response{OK: false, ReasonCode: "timeout", Detail: ctx.Err().Error()}
	}
	if request.Op != elasticCommandOperation {
		return elasticdaemon.Response{OK: false, ReasonCode: "operation_not_supported", Detail: request.Op}
	}
	var command model.CommandRequest
	if err := json.Unmarshal(request.Input, &command); err != nil {
		return elasticdaemon.Response{OK: false, ReasonCode: "request_invalid", Detail: "invalid mi-lsp command payload"}
	}

	started := time.Now()
	workersInUse, workerSlots := 0, 0
	if s.manager != nil {
		workersInUse, workerSlots = s.manager.WorkerCapacity()
	}
	response, err := s.handleRequestContext(ctx, command)
	if err != nil {
		response = daemonErrorEnvelope(command, err, "daemon")
	}
	elapsed := time.Since(started)
	if response.Ok && workerSlots > 0 && workersInUse >= workerSlots {
		if response.Metrics == nil {
			response.Metrics = &model.EnvelopeMetrics{}
		}
		response.Metrics.WorkersInUse = workersInUse
		response.Metrics.WorkerSlots = workerSlots
		response.Metrics.WorkersAtCapacity = true
		response.Warnings = append(response.Warnings, fmt.Sprintf("workers_at_capacity: in_use=%d max=%d latency_ms=%d", workersInUse, workerSlots, elapsed.Milliseconds()))
	}
	s.recordAccess(command, response, err, elapsed)
	// Domain errors remain successful transport responses with their original
	// mi-lsp envelope. Only the shared daemon's admission failures use reason_code.
	return elasticdaemon.Response{OK: true, Data: response}
}

func (s *Server) submitElastic(ctx context.Context, command model.CommandRequest) model.Envelope {
	input, err := json.Marshal(command)
	if err != nil {
		return daemonErrorEnvelope(command, err, "daemon")
	}
	caller := elasticCaller(command.Context.ClientName, command.Context.SessionID)
	poolCtx, cancel := context.WithCancel(ctx)
	stopShutdownCancel := context.AfterFunc(s.elasticCtx, cancel)
	defer stopShutdownCancel()
	defer cancel()

	result := s.elastic.Submit(poolCtx, elasticdaemon.Request{
		Op:     elasticCommandOperation,
		Caller: caller,
		Input:  input,
	})
	if result.ReasonCode == "ocupado" {
		response := elasticBusyEnvelope(command)
		s.recordAccess(command, response, nil, 0)
		return response
	}
	if result.ReasonCode != "" {
		return daemonErrorEnvelope(command, fmt.Errorf("shared daemon %s: %v", result.ReasonCode, result.Detail), "daemon")
	}
	response, ok := result.Data.(model.Envelope)
	if !ok {
		return daemonErrorEnvelope(command, errors.New("shared daemon returned an invalid mi-lsp response"), "daemon")
	}
	return response
}

func elasticCaller(clientName, sessionID string) string {
	clientName = strings.TrimSpace(clientName)
	sessionID = strings.TrimSpace(sessionID)
	if clientName == "" {
		clientName = "manual-cli"
	}
	if sessionID == "" {
		return clientName
	}
	return clientName + ":" + sessionID
}

func elasticBusyEnvelope(request model.CommandRequest) model.Envelope {
	message := "daemon/backpressure_busy: shared request queue is full; retry after 100 ms"
	return model.Envelope{
		Ok:      false,
		Backend: "daemon",
		Items: []map[string]any{{
			"error_kind":     "daemon",
			"error_code":     "backpressure_busy",
			"operation":      request.Operation,
			"retry_after_ms": 100,
		}},
		Warnings: []string{message},
		Hint:     "retry after 100 ms",
		Error: &model.EnvelopeError{
			Kind:       "daemon",
			Code:       "backpressure_busy",
			Message:    message,
			Stage:      "backend",
			HintCode:   "backpressure_busy",
			Retryable:  true,
			ReasonCode: "ocupado",
			Detail:     "retry_after_ms=100",
		},
	}
}
