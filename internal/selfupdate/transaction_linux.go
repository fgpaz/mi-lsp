//go:build linux

package selfupdate

import "os"

func txBackup(target, backup string) error { return os.Link(target, backup) }

// POSIX unlink preserves the inode and does not interrupt an executing image.
func txBackupSafe(_ string, _ os.FileInfo) (bool, error) { return true, nil }
