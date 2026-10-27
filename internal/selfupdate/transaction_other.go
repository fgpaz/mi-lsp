//go:build !linux && !windows

package selfupdate

import "os"

func txBackup(target, backup string) error { return os.Link(target, backup) }

// Without reliable system-wide executable liveness evidence, preserve the backup.
func txBackupSafe(_ string, _ os.FileInfo) (bool, error) { return false, nil }
