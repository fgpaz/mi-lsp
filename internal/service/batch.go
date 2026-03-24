package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type batchOperation struct {
	ID     string         `json:"id"`
	Op     string         `json:"op"`
	Params map[string]any `json:"params"`
}

type batchResult struct {
	ID       string         `json:"id"`
	Op       string         `json:"op"`
	Envelope model.Envelope `json:"envelope"`
	Error    string         `json:"error,omitempty"`
}

func (a *App) batch(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()

	var ops []batchOperation
	var parseErr error

	// First check if operations are provided in payload
	if rawOps, ok := request.Payload["operations"].(string); ok && rawOps != "" {
		parseErr = json.Unmarshal([]byte(rawOps), &ops)
	} else {
		// Try to read from stdin
		limReader := io.LimitReader(os.Stdin, 10*1024*1024) // 10MB max
		data, err := io.ReadAll(limReader)
		if err != nil {
			return model.Envelope{}, fmt.Errorf("reading stdin: %w", err)
		}
		if len(data) == 0 {
			return model.Envelope{}, errors.New("operations JSON required via stdin or payload")
		}
		parseErr = json.Unmarshal(data, &ops)
	}

	if parseErr != nil {
		return model.Envelope{}, fmt.Errorf("parsing batch operations: %w", parseErr)
	}
	if len(ops) == 0 {
		return model.Envelope{}, errors.New("at least one operation is required")
	}

	sequential, _ := request.Payload["sequential"].(bool)

	results := make([]batchResult, len(ops))

	if sequential {
		for i, op := range ops {
			results[i] = a.executeBatchOp(ctx, op, request.Context)
		}
	} else {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4) // max 4 parallel workers
		for i, op := range ops {
			wg.Add(1)
			go func(idx int, operation batchOperation) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
					results[idx] = a.executeBatchOp(ctx, operation, request.Context)
				case <-ctx.Done():
					results[idx] = batchResult{ID: operation.ID, Op: operation.Op, Error: "cancelled"}
				}
			}(i, op)
		}
		wg.Wait()
	}

	failed := 0
	for _, r := range results {
		if r.Error != "" {
			failed++
		}
	}

	return model.Envelope{
		Ok:      true,
		Backend: "batch",
		Items:   results,
		Stats: model.Stats{
			Symbols: len(ops),
			Files:   len(ops) - failed,
			Ms:      time.Since(started).Milliseconds(),
		},
	}, nil
}

func (a *App) executeBatchOp(ctx context.Context, op batchOperation, queryOpts model.QueryOptions) batchResult {
	id := op.ID
	if id == "" {
		id = op.Op
	}

	subRequest := model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       op.Op,
		Context:         queryOpts,
		Payload:         op.Params,
	}

	envelope, err := a.Execute(ctx, subRequest)
	result := batchResult{
		ID:       id,
		Op:       op.Op,
		Envelope: envelope,
	}
	if err != nil {
		result.Error = err.Error()
		result.Envelope = model.Envelope{Ok: false}
	}
	return result
}
