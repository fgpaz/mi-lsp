package daemon

import (
	"context"
	"errors"
	"time"
)

// telemetryOwnershipBudget bounds only owner/path/lease acquisition and WAL
// measurement. It never releases physical ownership while SQL is running.
const telemetryOwnershipBudget = 250 * time.Millisecond

func (s *TelemetryStore) telemetryOwnershipGate() chan struct{} {
	s.ownershipOnce.Do(func() { s.ownership = make(chan struct{}, 1) })
	return s.ownership
}

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
	acquireCtx, cancel := context.WithTimeout(ctx, telemetryOwnershipBudget)
	defer cancel()
	gate := s.telemetryOwnershipGate()
	select {
	case gate <- struct{}{}:
	case <-acquireCtx.Done():
		return acquireCtx.Err()
	}
	defer func() { <-gate }()
	return s.withTelemetryWriterLeaseContext(acquireCtx, func(context.Context) error {
		// Use caller lifetime for physical SQL, not the bounded acquire context.
		return fn(ctx)
	})
}

type telemetryAdmissionError struct{ cause error }

func (e telemetryAdmissionError) Error() string { return e.cause.Error() }
func (e telemetryAdmissionError) Unwrap() error { return e.cause }

func isTelemetryAdmissionError(err error) bool {
	var admissionErr telemetryAdmissionError
	return errors.As(err, &admissionErr)
}

func (s *TelemetryStore) withTelemetryMutationContext(ctx context.Context, fn func(context.Context) error) error {
	return s.withTelemetryMutationDrainContext(ctx, false, fn)
}

func (s *TelemetryStore) withTelemetryMutationDrainContext(ctx context.Context, allowClosing bool, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("telemetry mutation callback is nil")
	}
	admissionEntered := false
	err := s.withTelemetryWriteOwnershipContext(ctx, func(writeCtx context.Context) error {
		admissionEntered = true
		if !allowClosing {
			s.stateMu.RLock()
			closed := s.closed
			s.stateMu.RUnlock()
			if closed {
				err := errors.New("telemetry store closed")
				s.reject("closed")
				return telemetryAdmissionError{cause: err}
			}
		}
		if err := s.admitTelemetryMutationWithLeaseHeldContext(writeCtx); err != nil {
			return telemetryAdmissionError{cause: err}
		}
		return fn(writeCtx)
	})
	if err != nil && !admissionEntered {
		s.reject(walAdmissionReason(err))
		return telemetryAdmissionError{cause: err}
	}
	return err
}

// The caller must hold both the local ownership gate and canonical OS lease.
// Each refusal increments RejectedWrites exactly once here.
func (s *TelemetryStore) admitTelemetryMutationWithLeaseHeldContext(ctx context.Context) error {
	maxBytes := s.walMaxBytes
	if maxBytes <= 0 {
		maxBytes = telemetryWALMaxBytes
	}
	measureCtx, cancel := context.WithTimeout(ctx, telemetryOwnershipBudget)
	defer cancel()
	size, err := s.walSizeWithContext(measureCtx)
	if err == nil && size >= maxBytes {
		err = s.checkpointWithLeaseHeldContext(measureCtx)
		if err == nil {
			size, err = s.walSizeWithContext(measureCtx)
		}
		if err == nil && size >= maxBytes {
			err = errTelemetryWALLimit
		}
	}
	if err != nil {
		s.reject(walAdmissionReason(err))
	}
	return err
}

// withTelemetryReadOwnershipContext admits external readers under the same
// local/OS owner as SQL writers and holds it until rows/connections are closed.
func (s *TelemetryStore) withTelemetryReadOwnershipContext(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("telemetry read callback is nil")
	}
	return s.withTelemetryWriteOwnershipContext(ctx, func(ownedCtx context.Context) error {
		s.stateMu.RLock()
		closed := s.closed
		s.stateMu.RUnlock()
		if closed {
			return errors.New("telemetry store closed")
		}
		return fn(ownedCtx)
	})
}

func (s *TelemetryStore) withTelemetryOwnershipOnlyContext(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("telemetry ownership callback is nil")
	}
	return s.withTelemetryWriteOwnershipContext(ctx, fn)
}

// Closing is a physical ownership operation: do not abandon it on the caller's
// drain deadline. Wait for both gates with a live context until DB.Close returns.
func (s *TelemetryStore) withTelemetryCloseOwnership(fn func() error) error {
	if s == nil || s.db == nil {
		return nil
	}
	gate := s.telemetryOwnershipGate()
	gate <- struct{}{}
	defer func() { <-gate }()
	return s.withTelemetryWriterLeaseContext(context.Background(), func(context.Context) error { return fn() })
}
