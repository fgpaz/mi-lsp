package daemon

import (
 "context"
 "errors"
 "fmt"
 "os"
 "testing"
)

func TestWALAdmissionMeasurementPathIsNotBusy(t *testing.T) {
 // Error value models a failed metadata stat, not an actual ACL mutation.
 measurement := &os.PathError{Op: "stat", Path: "private/sqlite_busy-wal", Err: os.ErrNotExist}
 cases := []struct { err error; want string }{
  {measurement, "wal_admission_error"},
  {fmt.Errorf("metadata: %w", measurement), "wal_admission_error"},
  {errors.New("database is locked"), "wal_admission_error"},
  {fmt.Errorf("metadata: %w", context.DeadlineExceeded), "deadline"},
  {context.Canceled, "canceled"},
  {fmt.Errorf("checkpoint: %w", errTelemetryCheckpointBusy), "busy"},
  {errTelemetryWALLimit, "wal_limit"},
 }
 for _, c := range cases {
  if got := walAdmissionReason(c.err); got != c.want { t.Errorf("reason(%v) = %q, want %q", c.err, got, c.want) }
 }
}
