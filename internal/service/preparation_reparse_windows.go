//go:build windows

package service

import (
	"golang.org/x/sys/windows"
	"path/filepath"
)

func preparationPathReparse(path string) (bool, error) {
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
