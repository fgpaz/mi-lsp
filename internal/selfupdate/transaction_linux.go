//go:build linux

package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

func txBackup(target, backup string) error { return os.Link(target, backup) }

// Linux exposes executable inode identities. Unobservable processes force deferral.
func txBackupSafe(_ string, backupInfo os.FileInfo) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		info, err := os.Stat(filepath.Join("/proc", entry.Name(), "exe"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return false, err
		}
		if os.SameFile(backupInfo, info) {
			return false, nil
		}
	}
	return true, nil
}
