//go:build windows

package daemon

import (
	"unsafe"

	"github.com/fgpaz/mi-lsp/internal/model"
	"golang.org/x/sys/windows"
)

var (
	modPsapi                  = windows.NewLazySystemDLL("psapi.dll")
	modKernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessMemoryInfo  = modPsapi.NewProc("GetProcessMemoryInfo")
	procGetProcessHandleCount = modKernel32.NewProc("GetProcessHandleCount")
)

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func processMemoryBytes(pid int) uint64 {
	return processStats(pid).WorkingSetBytes
}

func processStats(pid int) model.DaemonProcessStats {
	stats := model.DaemonProcessStats{PID: pid}
	if pid == 0 {
		return stats
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return stats
	}
	defer windows.CloseHandle(handle)
	var counters processMemoryCounters
	counters.CB = uint32(unsafe.Sizeof(counters))
	ret, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.CB),
	)
	if ret != 0 {
		stats.WorkingSetBytes = uint64(counters.WorkingSetSize)
		stats.PrivateBytes = uint64(counters.PagefileUsage)
	}
	var handleCount uint32
	ret, _, _ = procGetProcessHandleCount.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&handleCount)),
	)
	if ret != 0 {
		stats.HandleCount = uint64(handleCount)
	}
	stats.ThreadCount = processThreadCount(uint32(pid))
	return stats
}

func processThreadCount(pid uint32) uint64 {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return 0
	}
	var count uint64
	for {
		if entry.OwnerProcessID == pid {
			count++
		}
		entry.Size = uint32(unsafe.Sizeof(windows.ThreadEntry32{}))
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			break
		}
	}
	return count
}
