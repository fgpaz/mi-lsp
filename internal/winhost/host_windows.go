//go:build windows

package winhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/fgpaz/mi-lsp/internal/model"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	createSuspended = 0x00000004
	createNoWindow = 0x08000000
	extendedStartupInfo = 0x00080000
	startfUseStdHandles = 0x00000100
	procThreadAttributeHandleList = 0x00020002
	duplicateSameAccess = 0x00000002
	waitObject0 = 0
	waitTimeout = 258
	cleanupWaitMS = 1000
)

var (
	kernel = syscall.NewLazyDLL("kernel32.dll")
	createJob = kernel.NewProc("CreateJobObjectW")
	assignJob = kernel.NewProc("AssignProcessToJobObject")
	terminateJob = kernel.NewProc("TerminateJobObject")
	queryJob = kernel.NewProc("QueryInformationJobObject")
	setJob = kernel.NewProc("SetInformationJobObject")
	setErrorMode = kernel.NewProc("SetErrorMode")
	resumeThread = kernel.NewProc("ResumeThread")
	initAttributes = kernel.NewProc("InitializeProcThreadAttributeList")
	updateAttribute = kernel.NewProc("UpdateProcThreadAttribute")
	deleteAttributes = kernel.NewProc("DeleteProcThreadAttributeList")
)

// assignOwnedProcess is private so the suspended-assignment failure path can
// be exercised without changing the job of the test runner.
var assignOwnedProcess = func(job, process uintptr) error {
	ok, _, err := assignJob.Call(job, process)
	if ok == 0 {
		return err
	}
	return nil
}

type startupInfoEx struct {
	startup syscall.StartupInfo
	attributes uintptr
}

// JOB_OBJECT_BASIC_ACCOUNTING_INFORMATION, without pointer-sized fields.
type jobAccounting struct {
	userTime, kernelTime, periodUserTime, periodKernelTime int64
	pageFaults, totalProcesses, activeProcesses, terminatedProcesses uint32
}

type jobBasicLimit struct {
	processTime, jobTime int64
	flags uint32
	minWorkingSet, maxWorkingSet uintptr
	activeLimit uint32
	affinity uintptr
	priority, scheduling uint32
}
type ioCounters struct { readOps, writeOps, otherOps, readBytes, writeBytes, otherBytes uint64 }
type jobExtendedLimit struct {
	basic jobBasicLimit
	io ioCounters
	processMemory, jobMemory, peakProcessMemory, peakJobMemory uintptr
}

type diagnostic struct {
	Operation string `json:"operation"`
	Win32 uint32 `json:"win32,omitempty"`
}

type outcome struct {
	Protocol string `json:"protocol"`
	Kind string `json:"kind"`
	ExitCode uint32 `json:"exit_code"`
	RootExitCode *uint32 `json:"root_exit_code,omitempty"`
	Signal string `json:"signal,omitempty"`
	Error *diagnostic `json:"error,omitempty"`
	Cleanup []diagnostic `json:"cleanup_errors,omitempty"`
}

func diag(op string, err error) diagnostic {
	var errno syscall.Errno
	errors.As(err, &errno)
	return diagnostic{Operation: op, Win32: uint32(errno)}
}

func failed(op string, err error) outcome {
	d := diag(op, err)
	return outcome{Protocol: "execsafe-host/v1", Kind: "infra_error", ExitCode: 125, Error: &d}
}

func waitRoot(handle syscall.Handle) error {
	state, err := syscall.WaitForSingleObject(handle, cleanupWaitMS)
	if err != nil { return err }
	if state != waitObject0 { return syscall.ETIMEDOUT }
	return nil
}

func drainJob(job uintptr) error {
	deadline := time.Now().Add(time.Duration(cleanupWaitMS) * time.Millisecond)
	for {
		var info jobAccounting
		ok, _, err := queryJob.Call(job, 1, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0)
		if ok == 0 { return err }
		if info.activeProcesses == 0 { return nil }
		if !time.Now().Before(deadline) { return syscall.ETIMEDOUT }
		time.Sleep(10 * time.Millisecond)
	}
}

func run(args []string, timeout time.Duration) (result outcome) {
	deadline := time.Now().Add(timeout)
	result = failed("invalid_invocation", nil)
	if len(args) < 4 || args[0] != "--control-file" || args[2] != "--" || !filepath.IsAbs(args[1]) || !filepath.IsAbs(args[3]) || !strings.EqualFold(filepath.Ext(args[3]), ".exe") {
		return result
	}
	control, err := os.Open(args[1])
	if err != nil { return failed("open_control", err) }
	defer func() {
		if err := control.Close(); err != nil { result.Cleanup = append(result.Cleanup, diag("close_control", err)) }
	}()
	stat, err := control.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != 0 { return failed("control_shape", err) }
	job, _, e := createJob.Call(0, 0)
	if job == 0 { return failed("create_job", e) }
	defer func() {
		if err := syscall.CloseHandle(syscall.Handle(job)); err != nil {
			result.Cleanup = append(result.Cleanup, diag("close_job", err))
		}
	}()

	limits := jobExtendedLimit{}
	limits.basic.flags = 0x2000 // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if ok, _, e := setJob.Call(job, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits)); ok == 0 { return failed("configure_job", e) }

	// Duplicates are inheritable; the original caller handles are never changed.
	// The job, control and result handles are NOT in the explicit inheritance list.
	current := syscall.Handle(^uintptr(0))
	handles := make([]syscall.Handle, 3)
	for i, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if f == nil { return failed("missing_stdio", nil) }
		if err := syscall.DuplicateHandle(current, syscall.Handle(f.Fd()), current, &handles[i], 0, true, duplicateSameAccess); err != nil {
			return failed("duplicate_stdio", err)
		}
		h := handles[i]
		defer func() {
			if err := syscall.CloseHandle(h); err != nil { result.Cleanup = append(result.Cleanup, diag("close_stdio", err)) }
		}()
	}
	var size uintptr
	initAttributes.Call(0, 1, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 { return failed("attribute_size", nil) }
	storage := make([]uintptr, (size + unsafe.Sizeof(uintptr(0)) - 1) / unsafe.Sizeof(uintptr(0)))
	attrs := uintptr(unsafe.Pointer(&storage[0]))
	r, _, e := initAttributes.Call(attrs, 1, 0, uintptr(unsafe.Pointer(&size)))
	if r == 0 { return failed("initialize_attributes", e) }
	defer func() {
		deleteAttributes.Call(attrs)
		runtime.KeepAlive(storage)
		runtime.KeepAlive(handles)
	}()
	r, _, e = updateAttribute.Call(attrs, 0, procThreadAttributeHandleList, uintptr(unsafe.Pointer(&handles[0])), uintptr(len(handles)) * unsafe.Sizeof(handles[0]), 0, 0)
	if r == 0 { return failed("inheritance_list", e) }
	app, err := syscall.UTF16PtrFromString(args[3])
	if err != nil { return failed("executable_encoding", err) }
	quoted := make([]string, 0, len(args)-3)
	for _, arg := range args[3:] { quoted = append(quoted, syscall.EscapeArg(arg)) }
	line, err := syscall.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil { return failed("argv_encoding", err) }
	startup := startupInfoEx{attributes: attrs}
	startup.startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.startup.Flags = startfUseStdHandles
	startup.startup.StdInput, startup.startup.StdOutput, startup.startup.StdErr = handles[0], handles[1], handles[2]
	var process syscall.ProcessInformation
	err = syscall.CreateProcess(app, line, nil, nil, true, createSuspended | createNoWindow | extendedStartupInfo, nil, nil, &startup.startup, &process)
	if err != nil { return failed("create_process", err) }
	defer func() {
		if err := syscall.CloseHandle(process.Process); err != nil { result.Cleanup = append(result.Cleanup, diag("close_process", err)) }
		if err := syscall.CloseHandle(process.Thread); err != nil { result.Cleanup = append(result.Cleanup, diag("close_thread", err)) }
	}()
	assigned, completed := false, false
	var rootCode *uint32
	defer func() {
		if rootCode != nil && result.RootExitCode == nil { result.RootExitCode = rootCode }
		if result.Kind != "infra_error" || completed { return }
		// No PID reopen, and no unbounded cleanup wait. A failed cleanup remains
		// explicitly failed, never reported as successful cancellation/reaping.
		if !assigned {
			if err := syscall.TerminateProcess(process.Process, 125); err != nil { result.Cleanup = append(result.Cleanup, diag("terminate_suspended_root", err)) }
		} else {
			if ok, _, err := terminateJob.Call(job, 125); ok == 0 { result.Cleanup = append(result.Cleanup, diag("terminate_cleanup_job", err)) }
		}
		if err := waitRoot(process.Process); err != nil { result.Cleanup = append(result.Cleanup, diag("reap_cleanup_root", err)) }
		if assigned {
			if err := drainJob(job); err != nil { result.Cleanup = append(result.Cleanup, diag("drain_cleanup_job", err)) }
		}
	}()
	if err := assignOwnedProcess(job, uintptr(process.Process)); err != nil { return failed("assign_job", err) }
	assigned = true
	r, _, e = resumeThread.Call(uintptr(process.Thread))
	if uint32(r) == 0xffffffff { return failed("resume_thread", e) }
	for {
		state, err := syscall.WaitForSingleObject(process.Process, 0)
		if err != nil { return failed("wait_root", err) }
		if state == waitObject0 && rootCode == nil {
			var code uint32
			if err := syscall.GetExitCodeProcess(process.Process, &code); err != nil { return failed("read_root_exit", err) }
			rootCode = &code
		} else if state != waitObject0 && state != waitTimeout { return failed("root_wait_state", nil) }
		var info jobAccounting
		if ok, _, e := queryJob.Call(job, 1, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0); ok == 0 { return failed("query_job", e) }
		if rootCode != nil && info.activeProcesses == 0 {
			completed = true
			return outcome{Protocol: "execsafe-host/v1", Kind: "root_exit", ExitCode: *rootCode, RootExitCode: rootCode}
		}
		if !time.Now().Before(deadline) { return failed("deadline", nil) }
		var buf [17]byte
		n, readErr := control.ReadAt(buf[:], 0)
		if readErr != nil && readErr != io.EOF { return failed("read_control", readErr) }
		token := string(buf[:n])
		// Support existing no-newline shell tokens; an in-progress prefix is
		// pending, not a malformed command which would kill the owned job.
		if token == "" { time.Sleep(10 * time.Millisecond); continue }
		signal := strings.TrimSuffix(token, "\n")
		if signal != "INT" && signal != "TERM" {
			if strings.HasPrefix("INT\n", token) || strings.HasPrefix("TERM\n", token) { time.Sleep(10 * time.Millisecond); continue }
			return failed("control_protocol", nil)
		}
		// The final job-empty observation linearizes completion versus cancellation.
		if ok, _, e := queryJob.Call(job, 1, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0); ok == 0 { return failed("cancel_query_job", e) }
		if info.activeProcesses == 0 { continue }
		code := uint32(130)
		if signal == "TERM" { code = 143 }
		r, _, e = terminateJob.Call(job, uintptr(code))
		if r == 0 { return failed("terminate_job", e) }
		if err := waitRoot(process.Process); err != nil { return failed("reap_cancel_root", err) }
		if err := drainJob(job); err != nil { return failed("drain_cancel_job", err) }
		completed = true
		return outcome{Protocol: "execsafe-host/v1", Kind: "cancelled", Signal: signal, ExitCode: code, RootExitCode: rootCode}
	}
}

func execute(args []string) uint32 {
	timeout := 300000
	if len(args) >= 2 && args[0] == "--timeout-ms" {
		s := args[1]
		if s == "" || strings.Trim(s, "0123456789") != "" { return 125 }
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 86400000 { return 125 }
		timeout = n
		args = args[2:]
	}
	if len(args) < 2 || args[0] != "--control-file" || !filepath.IsAbs(args[1]) { return 125 }
	// The caller owns this private directory and stable control-file identity.
	// Exclusive creation cannot overwrite a pre-existing file or follow its link.
	// No ACL/chmod operation is used. The result is IPC, not an authority grant.
	file, err := os.OpenFile(args[1] + ".result", os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0600)
	if err != nil { return 125 }
	result := run(args, time.Duration(timeout)*time.Millisecond)
	if len(result.Cleanup) != 0 && result.Kind != "infra_error" {
		result.Kind, result.ExitCode = "infra_error", 125
		d := diag("cleanup_failed", nil)
		result.Error = &d
	}
	return writeResult(file, result)
}

// This boundary is independently fault-injectable without creating a process.
// It does not certify native filesystem behaviour or Windows no-flash.
func writeResult(file io.WriteCloser, result outcome) uint32 {
	body, err := json.Marshal(result)
	if err != nil { file.Close(); return 125 }
	// A fixed ASCII first line supports shell builtin read without launching an
	// interpreter. A second JSON line carries structured public diagnostics.
	header := fmt.Sprintf("execsafe-host/v1 %s %d\n", result.Kind, result.ExitCode)
	payload := append([]byte(header), append(body, '\n')...)
	n, err := file.Write(payload)
	closeErr := file.Close()
	if err != nil || n != len(payload) || closeErr != nil { return 125 }
	// Transport success is separate from the payload result. Otherwise a
	// result-file close failure125 could masquerade as a real root exit125.
	// The adapter must propagate the typed record, not this transport zero.
	return 0
}

func Main(args []string) int {
	setErrorMode.Call(0x0001 | 0x0002 | 0x8000)
	if len(args) == 3 && args[0] == "--version" && args[1] == "--format" && args[2] == "json" {
		v := struct { Tool string `json:"tool"`; Version string `json:"version"`; Revision string `json:"revision"`; ABI string `json:"abi"`; Arch string `json:"arch"`; JobPolicy string `json:"job_policy"` }{"mi-lsp-host", model.BuildVersion, model.BuildRevision, "execsafe-host/v1", runtime.GOARCH, "kill_on_close_wait_tree"}
		if err := json.NewEncoder(os.Stdout).Encode(v); err != nil { return 125 }; return 0
	}
	return int(execute(args))
}
