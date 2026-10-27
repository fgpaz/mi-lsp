//go:build !linux && !windows

package selfupdate

import "os"

func txBackup(target, backup string) error { return os.Link(target, backup) }

// POSIX unlink does not terminate processes executing the backed-up inode.
func txBackupSafe(_ string, _ os.FileInfo) (bool, error) { return true, nil }
