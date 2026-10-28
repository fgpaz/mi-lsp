package daemon

import (
	"context"
	"errors"
	"time"
)

// telemetryOwnershipBudget limits only local gate/OS-lease acquisition and
// legacy database-path resolution. It never bounds the callback while it owns
// the physical writer lease.
const telemetryOwnershipBudget = 250 * time.Millisecond

func (s *TelemetryStore) withTelemetryWriteOwnershipContext(ctx context.Context, fn func(context.Context) error) error {
	if s == nil || s.db == nil {
		return errors.New("telemetry store unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return errors.New("telemetry ownership callback is nil")
	}
	if s.ownership == nil {
		s.stateMu.Lock()
		if s.ownership == nil {
			s.ownership = make(chan struct{}, 1)
		}
		s.stateMu.Unlock()
	}
	acquireCtx, cancel := context.WithTimeout(ctx, telemetryOwnershipBudget)
	defer cancel()
	select {
	case s.ownership <- struct{}{}:
	case <-acquireCtx.Done():
		return acquireCtx.Err()
	}
	defer func() { <-s.ownership }()
	return s.withTelemetryWriterLeaseContext(acquireCtx, func(context.Context) error {
		// Callback gets its original, independently-lived context. The bounded
		// context above is solely for gate/lease/path resolution.
		return fn(ctx)
	})
}

func (s *TelemetryStore) withTelemetryMutationContext(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("telemetry mutation callback is nil")
	}
	return s.withTelemetryWriteOwnershipContext(ctx, func(writeCtx context.Context) error {
		if err := s.admitTelemetryMutationWithLeaseHeldContext(writeCtx); err != nil {
			s.reject(walAdmissionReason(err))
			return err
		}
		return fn(writeCtx)
	})
}

// admitTelemetryMutationWithLeaseHeldContext requires both the local owner and
// the canonical database OS lease. It does not acquire either lock itself.
func (s *TelemetryStore) admitTelemetryMutationWithLeaseHeldContext(ctx context.Context) error {
	maxBytes := s.walMaxBytes
	if maxBytes <= 0 {
		maxBytes = telemetryWALMaxBytes
	}
	measureCtx, cancel := context.WithTimeout(ctx, telemetryOwnershipBudget)
	defer cancel()
	size, err := s.walSizeWithContext(measureCtx)
	if err != nil {
		return err
	}
	if size >= maxBytes {
		if err := s.checkpointWithLeaseHeldContext(measureCtx); err != nil {
			return err
		}
		size, err = s.walSizeWithContext(measureCtx)
		if err != nil {
			return err
		}
		if size >= maxBytes {
			return errTelemetryWALLimit
		}
	}
	return nil
}

func (s *TelemetryStore) withTelemetryOwnershipOnlyContext(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("telemetry ownership callback is nil")
	}
	return s.withTelemetryWriteOwnershipContext(ctx, fn)
}

// Physical close may not abandon acquisition when its caller's drain deadline
// expires. It waits for both ownership layers with a live background context.
func (s *TelemetryStore) withTelemetryCloseOwnership(fn func() error) error {
	if s == nil || s.db == nil {
		return nil
	}
	if s.ownership == nil {
		s.stateMu.Lock()
		if s.ownership == nil {
			s.ownership = make(chan struct{}, 1)
		}
		s.stateMu.Unlock()
	}
	s.ownership <- struct{}{}
	defer func() { <-s.ownership }()
	return s.withTelemetryWriterLeaseContext(context.Background(), func(context.Context) error { return fn() })
}
