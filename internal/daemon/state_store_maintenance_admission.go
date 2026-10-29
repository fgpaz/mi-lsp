package daemon

import (
	"context"
	"fmt"
	"time"
)

// purgeAndVacuumContext keeps the OS writer lease for the whole maintenance
// operation. Each distinct mutation is admitted immediately before it runs.
func (s *TelemetryStore) purgeAndVacuumContext(ctx context.Context, retentionDays int, maxBytes int64) (int, bool, error) {
	var purged int64
	var vacuumed bool
	err := s.withTelemetryWriteOwnershipContext(ctx, func(ownedCtx context.Context) error {
		days := retentionDays
		// maxBytes selects the legacy aggressive-retention window only; it is
		// neither a VACUUM toggle nor a WAL budget.
		sz, sizeErr := s.dbSize()
		if sizeErr == nil && sz > maxBytes {
			days = 7
		}
		cutoff := time.Now().AddDate(0, 0, -days)
		var err error
		purged, err = s.purgeOldEventsWithLeaseHeldContext(ownedCtx, cutoff)
		if err != nil {
			return fmt.Errorf("purge events: %w", err)
		}
		if err := s.vacuumWithLeaseHeldContext(ownedCtx); err != nil {
			return fmt.Errorf("vacuum: %w", err)
		}
		vacuumed = true
		return nil
	})
	return int(purged), vacuumed, err
}

func (s *TelemetryStore) purgeOldEventsContext(ctx context.Context, olderThan time.Time) (int64, error) {
	var purged int64
	err := s.withTelemetryMutationContext(ctx, func(ownedCtx context.Context) error {
		var err error
		purged, err = s.purgeOldEventsSQLContext(ownedCtx, olderThan)
		return err
	})
	return purged, err
}

func (s *TelemetryStore) purgeOldEventsWithLeaseHeldContext(ctx context.Context, olderThan time.Time) (int64, error) {
	if err := s.admitTelemetryMutationWithLeaseHeldContext(ctx); err != nil {
		return 0, err
	}
	return s.purgeOldEventsSQLContext(ctx, olderThan)
}

func (s *TelemetryStore) purgeOldEventsSQLContext(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.execWithRetryContext(ctx, `DELETE FROM access_events WHERE occurred_at < ?`, olderThan.Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *TelemetryStore) purgeOldRunsContext(ctx context.Context, olderThan time.Time) (int64, error) {
	var purged int64
	err := s.withTelemetryMutationContext(ctx, func(ownedCtx context.Context) error {
		var err error
		purged, err = s.purgeOldRunsSQLContext(ownedCtx, olderThan)
		return err
	})
	return purged, err
}

func (s *TelemetryStore) purgeOldRunsSQLContext(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.execWithRetryContext(ctx, `DELETE FROM daemon_runs WHERE stopped_at IS NOT NULL AND stopped_at < ?`, olderThan.Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// vacuumWithLeaseHeldContext is the single admitted VACUUM phase. Callers of
// this helper must already own the store and OS writer lease.
func (s *TelemetryStore) vacuumWithLeaseHeldContext(ctx context.Context) error {
	vctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.admitTelemetryMutationWithLeaseHeldContext(vctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(vctx, "VACUUM")
	return err
}
