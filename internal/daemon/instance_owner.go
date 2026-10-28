package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func daemonInstanceLockPath() (string, error) {
	dir, err := daemonRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "instance.lock"), nil
}

// ErrDaemonInaccessible means a recorded owner may be alive but communication
// failed; it is not evidence that the daemon is absent.
var ErrDaemonInaccessible = errors.New("daemon_inaccessible: owner is live or indeterminate; use an ordinary same-user session and inspect daemon integrity; do not terminate another owner's daemon")

func ClassifyDaemonStatusError(cause error) error {
	state, err := loadDaemonState()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: owner state unavailable", ErrDaemonInaccessible)
		}
		return BuildStatusError()
	}
	if state.PID <= 0 || uint64(state.PID) > startLockMaxPID || processExists(state.PID) {
		return fmt.Errorf("%w: communication failed", ErrDaemonInaccessible)
	}
	return BuildStatusError()
}

// Reject any recorded owner that is live or cannot safely be proved dead.
func rejectExistingDaemonOwner() error {
	state, err := loadDaemonState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("daemon owner state is unreadable; refusing startup: %w", err)
	}
	if state.PID <= 0 || uint64(state.PID) > startLockMaxPID {
		return errors.New("daemon owner state has invalid PID; refusing startup")
	}
	if processExists(state.PID) {
		return fmt.Errorf("%w: refusing startup", ErrDaemonInaccessible)
	}
	return nil
}
