//go:build windows

package daemon

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

func checkDaemonStartIntegrity() error {
	var elevation uint32
	var size uint32
	err := windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &size)
	if err != nil || size != uint32(unsafe.Sizeof(elevation)) {
		return errors.New("daemon_integrity_unknown: cannot verify token elevation; start from an ordinary user session")
	}
	if elevation != 0 {
		return errors.New("daemon_elevated_forbidden: start daemon from an ordinary non-elevated user session")
	}
	return nil
}
