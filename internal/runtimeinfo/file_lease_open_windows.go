//go:build windows

package runtimeinfo

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type fileAttributeTagInfo struct { FileAttributes uint32; ReparseTag uint32 }

func openFileLeaseGuard(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil { return nil, err }
	// Deliberately omit FILE_SHARE_DELETE so this handle's pathname cannot be
	// replaced or removed while the lease is held. A nil SecurityAttributes
	// also requests a non-inheritable handle.
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.SECURITY_SQOS_PRESENT, 0)
	if err != nil { return nil, err }
	var info fileAttributeTagInfo
	err = windows.GetFileInformationByHandleEx(h, windows.FileAttributeTagInfo, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil { _ = windows.CloseHandle(h); return nil, err }
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		_ = windows.CloseHandle(h)
		return nil, windows.ERROR_INVALID_REPARSE_DATA
	}
	return os.NewFile(uintptr(h), path), nil
}
