package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const utilityRetention = 30 * 24 * time.Hour

const utilityTimestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

func utilityTimestamp(value time.Time) string {
	return value.UTC().Format(utilityTimestampLayout)
}

func RecordUtilityEvent(ctx context.Context, db *sql.DB, event model.UtilityEvent) error {
	if ctx == nil || db == nil {
		return model.ErrGraphGenerationInvalid
	}
	event, ok := event.Normalize()
	if !ok || event.CandidateNodeKey == "" {
		return errors.New("invalid candidate-scoped utility event")
	}
	when := event.OccurredAt.UTC()
	cutoff := when.Add(-utilityRetention)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(err error) error { _ = tx.Rollback(); return err }
	if _, err := tx.ExecContext(ctx, `DELETE FROM utility_events WHERE workspace_scope=? AND intent=? AND operation=? AND occurred_at < ?`, event.WorkspaceScope, event.Intent, event.Operation, utilityTimestamp(cutoff)); err != nil {
		return rollback(err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM utility_events WHERE workspace_scope=? AND intent=? AND operation=? AND candidate_node_key=?`, event.WorkspaceScope, event.Intent, event.Operation, event.CandidateNodeKey).Scan(&count); err != nil {
		return rollback(err)
	}
	if count >= model.UtilityMaxEventsPerCandidate {
		remove := count - model.UtilityMaxEventsPerCandidate + 1
		if _, err := tx.ExecContext(ctx, `DELETE FROM utility_events WHERE event_id IN (SELECT event_id FROM utility_events WHERE workspace_scope=? AND intent=? AND operation=? AND candidate_node_key=? ORDER BY occurred_at ASC,event_id ASC LIMIT ?)`, event.WorkspaceScope, event.Intent, event.Operation, event.CandidateNodeKey, remove); err != nil {
			return rollback(err)
		}
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM utility_events WHERE workspace_scope=? AND intent=? AND operation=?`, event.WorkspaceScope, event.Intent, event.Operation).Scan(&count); err != nil {
		return rollback(err)
	}
	if count >= model.UtilityMaxEventsPerScopeIntentOperation {
		remove := count - model.UtilityMaxEventsPerScopeIntentOperation + 1
		if _, err := tx.ExecContext(ctx, `DELETE FROM utility_events WHERE event_id IN (SELECT event_id FROM utility_events WHERE workspace_scope=? AND intent=? AND operation=? ORDER BY occurred_at ASC,event_id ASC LIMIT ?)`, event.WorkspaceScope, event.Intent, event.Operation, remove); err != nil {
			return rollback(err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO utility_events(workspace_scope,intent,operation,candidate_node_key,signal,value,generation_id,occurred_at) VALUES(?,?,?,?,?,?,?,?)`, event.WorkspaceScope, event.Intent, event.Operation, event.CandidateNodeKey, event.Signal, event.Value, event.GenerationID, utilityTimestamp(when))
	if err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func UtilityEvents(ctx context.Context, db *sql.DB, workspace, intent, operation, candidateNodeKey string, now time.Time) ([]model.UtilityEvent, error) {
	if ctx == nil || db == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	workspace, intent, operation, candidateNodeKey = strings.TrimSpace(workspace), model.SanitizeUtilityIntent(intent), strings.TrimSpace(operation), strings.TrimSpace(candidateNodeKey)
	rows, err := db.QueryContext(ctx, `SELECT workspace_scope,intent,operation,candidate_node_key,signal,value,generation_id,occurred_at FROM utility_events WHERE workspace_scope=? AND intent=? AND operation=? AND candidate_node_key=? AND occurred_at >= ? ORDER BY occurred_at DESC,event_id DESC LIMIT ?`, workspace, intent, operation, candidateNodeKey, utilityTimestamp(now.Add(-utilityRetention)), model.UtilityMaxEventsPerCandidate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.UtilityEvent, 0)
	for rows.Next() {
		var e model.UtilityEvent
		var occurred string
		if err := rows.Scan(&e.WorkspaceScope, &e.Intent, &e.Operation, &e.CandidateNodeKey, &e.Signal, &e.Value, &e.GenerationID, &occurred); err != nil {
			return nil, err
		}
		e.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func UtilitySignals(ctx context.Context, db *sql.DB, workspace, intent, operation string, now time.Time) (map[string]model.UtilitySignal, error) {
	if ctx == nil || db == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := db.QueryContext(ctx, `SELECT workspace_scope,intent,operation,candidate_node_key,signal,value,generation_id,occurred_at FROM utility_events WHERE workspace_scope=? AND intent=? AND operation=? AND occurred_at >= ? ORDER BY occurred_at DESC,event_id DESC LIMIT ?`, strings.TrimSpace(workspace), model.SanitizeUtilityIntent(intent), strings.TrimSpace(operation), utilityTimestamp(now.Add(-utilityRetention)), model.UtilityMaxEventsPerScopeIntentOperation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grouped := map[string][]model.UtilityEvent{}
	for rows.Next() {
		var e model.UtilityEvent
		var occurred string
		if err := rows.Scan(&e.WorkspaceScope, &e.Intent, &e.Operation, &e.CandidateNodeKey, &e.Signal, &e.Value, &e.GenerationID, &occurred); err != nil {
			return nil, err
		}
		if e.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred); err != nil {
			return nil, err
		}
		if len(grouped[e.CandidateNodeKey]) < model.UtilityMaxEventsPerCandidate {
			grouped[e.CandidateNodeKey] = append(grouped[e.CandidateNodeKey], e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make(map[string]model.UtilitySignal, len(grouped))
	for candidate, events := range grouped {
		result[candidate] = model.UtilityScoreForCandidate(events, workspace, intent, operation, candidate, now)
	}
	return result, nil
}

func UtilityScore(ctx context.Context, db *sql.DB, workspace, intent, operation, candidateNodeKey string, now time.Time) (model.UtilitySignal, error) {
	events, err := UtilityEvents(ctx, db, workspace, intent, operation, candidateNodeKey, now)
	if err != nil {
		return model.UtilitySignal{}, err
	}
	return model.UtilityScoreForCandidate(events, workspace, intent, operation, candidateNodeKey, now), nil
}
