//go:build windows

package workspace

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func canonPathIsReparse(path string) (bool, error) {
	name, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return false, err
	}
	attrs, err := windows.GetFileAttributes(name)
	if err != nil {
		return false, err
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}
