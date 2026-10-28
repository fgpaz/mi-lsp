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
		return fmt.Errorf("daemon owner PID %d is live or indeterminate; refusing startup", state.PID)
	}
	return nil
}

