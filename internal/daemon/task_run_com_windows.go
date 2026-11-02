//go:build windows
// +build windows

package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

const taskRunCOINITApartmentThreaded = 0x2

var (
	taskRunCoInitializeEx = syscall.NewLazyDLL("ole32.dll").NewProc("CoInitializeEx")
	taskRunCoUninitialize = syscall.NewLazyDLL("ole32.dll").NewProc("CoUninitialize")
)

type taskRunCOMSession struct {
	service *ole.IDispatch
	folder  *ole.IDispatch
	task         *ole.IDispatch
	runDispatchID int32
	locked       bool
	inited  bool
	closed  bool
}

func newTaskRunCOMSession() (taskRunSession, error) {
	runtime.LockOSThread()
	s := &taskRunCOMSession{locked: true}
	hr, _, _ := taskRunCoInitializeEx.Call(0, taskRunCOINITApartmentThreaded)
	// Both S_OK (0) and S_FALSE (1) mean this thread acquired one COM init
	// count and must balance it. A negative HRESULT, including changed mode,
	// did not acquire a count and must not be uninitialized.
	if int32(hr) < 0 {
		runtime.UnlockOSThread()
		s.locked = false
		return nil, fmt.Errorf("CoInitializeEx failed: HRESULT 0x%08x", uint32(hr))
	}
	s.inited = true
	return s, nil
}

func (s *taskRunCOMSession) Preflight(ctx context.Context, req taskRunRequest) error {
	if err := ctx.Err(); err != nil { return err }
	unknown, err := oleutil.CreateObject("Schedule.Service")
	if unknown != nil {
		defer unknown.Release() // Release returns a refcount, not an HRESULT.
	}
	if err != nil { return fmt.Errorf("create Schedule.Service: %w", err) }
	if unknown == nil { return errors.New("CreateObject returned nil IUnknown") }
	if err := ctx.Err(); err != nil { return err }
	dispatch, err := unknown.QueryInterface(ole.IID_IDispatch)
	if dispatch != nil { s.service = dispatch } // own before interpreting a paired error
	if err != nil { return fmt.Errorf("query Schedule.Service IDispatch: %w", err) }
	if dispatch == nil { return errors.New("Schedule.Service QueryInterface returned nil IDispatch") }
	if err := ctx.Err(); err != nil { return err }
	if err := callAndClear(s.service, "Connect"); err != nil { return fmt.Errorf("Task Scheduler Connect: %w", err) }
	if err := ctx.Err(); err != nil { return err }

	separator := strings.LastIndex(req.TaskPath, "\\")
	if separator < 0 || separator == len(req.TaskPath)-1 { return errors.New("invalid absolute Task path") }
	folderPath, taskName := req.TaskPath[:separator], req.TaskPath[separator+1:]
	if folderPath == "" { folderPath = "\\" }
	if err := ctx.Err(); err != nil { return err }
	fv, err := oleutil.CallMethod(s.service, "GetFolder", folderPath)
	if err != nil { return fmt.Errorf("GetFolder: %w", cleanupTaskRunVariants(err, fv)) }
	if err := ctx.Err(); err != nil { return cleanupTaskRunVariants(err, fv) }
	s.folder, err = takeDispatch(fv)
	if err != nil { return fmt.Errorf("GetFolder result: %w", err) }
	if err := ctx.Err(); err != nil { return err }
	tv, err := oleutil.CallMethod(s.folder, "GetTask", taskName)
	if err != nil { return fmt.Errorf("GetTask: %w", cleanupTaskRunVariants(err, tv)) }
	if err := ctx.Err(); err != nil { return cleanupTaskRunVariants(err, tv) }
	s.task, err = takeDispatch(tv)
	if err != nil { return fmt.Errorf("GetTask result: %w", err) }
	if err := ctx.Err(); err != nil { return err }
	xmlValue, err := oleutil.GetProperty(s.task, "XML")
	if err != nil { return fmt.Errorf("read Task XML: %w", cleanupTaskRunVariants(err, xmlValue)) }
	if err := ctx.Err(); err != nil { return cleanupTaskRunVariants(err, xmlValue) }
	if xmlValue == nil { return errors.New("Task XML returned nil VARIANT") }
	xml, vt := xmlValue.ToString(), xmlValue.VT
	clearErr := xmlValue.Clear()
	if clearErr != nil { return fmt.Errorf("clear Task XML VARIANT: %w", clearErr) }
	if vt != ole.VT_BSTR { return fmt.Errorf("Task XML has VARIANT type %d, want BSTR", vt) }
	sum := sha256.Sum256([]byte(xml))
	if hex.EncodeToString(sum[:]) != req.ExpectedXMLSHA256 { return errors.New("registered Task XML hash mismatch") }
	return ctx.Err()
}

func (s *taskRunCOMSession) PrepareRun(ctx context.Context) error {
	if err := ctx.Err(); err != nil { return err }
	dispatchIDs, err := s.task.GetIDsOfName([]string{"Run"})
	if err != nil { return fmt.Errorf("resolve Task Run dispatch ID: %w", err) }
	if err := ctx.Err(); err != nil { return err }
	if len(dispatchIDs) != 1 { return errors.New("Task Run dispatch lookup returned unexpected ID count") }
	s.runDispatchID = dispatchIDs[0]
	return nil
}

// Run enters the already-resolved IDispatch.Invoke path. Name lookup is done
// in PrepareRun, before cancellation can be frozen as an invocation attempt.
func (s *taskRunCOMSession) Run(nonce string) (string, error) {
	result, err := s.task.Invoke(s.runDispatchID, ole.DISPATCH_METHOD, nonce)
	if err != nil { return "", fmt.Errorf("Task Run: %w", cleanupTaskRunVariants(err, result)) }
	if result == nil { return "", errors.New("Task Run returned nil VARIANT") }
	if result.VT != ole.VT_DISPATCH {
		vt := result.VT
		clearErr := result.Clear()
		return "", errors.Join(fmt.Errorf("Task Run returned VARIANT type %d, want dispatch", vt), clearErr)
	}
	dispatch := result.ToIDispatch() // borrowed; result.Clear owns its release.
	if dispatch == nil { return "", errors.Join(errors.New("Task Run returned nil IDispatch"), cleanupTaskRunVariants(nil, result)) }
	guidValue, err := oleutil.GetProperty(dispatch, "InstanceGuid")
	if err != nil {
		return "", fmt.Errorf("read InstanceGuid: %w", cleanupTaskRunVariants(err, guidValue, result))
	}
	if guidValue == nil { return "", errors.Join(errors.New("InstanceGuid returned nil VARIANT"), cleanupTaskRunVariants(nil, result)) }
	guid, vt := guidValue.ToString(), guidValue.VT
	clearErr := errors.Join(guidValue.Clear(), result.Clear())
	if clearErr != nil { return "", fmt.Errorf("clear Task Run result VARIANTs: %w", clearErr) }
	if vt != ole.VT_BSTR { return "", fmt.Errorf("InstanceGuid has VARIANT type %d, want BSTR", vt) }
	if !validTaskInstanceGUID(guid) { return "", errors.New("Task Scheduler returned invalid InstanceGuid") }
	return guid, nil
}

func (s *taskRunCOMSession) Close() error {
	if s.closed { return errors.New("COM session closed twice") }
	s.closed = true
	for _, d := range []*ole.IDispatch{s.task, s.folder, s.service} {
		if d != nil { d.Release() } // go-ole exposes only the COM refcount here.
	}
	if s.inited { taskRunCoUninitialize.Call(); s.inited = false }
	if s.locked { runtime.UnlockOSThread(); s.locked = false }
	return nil
}

func callAndClear(d *ole.IDispatch, name string) error {
	v, err := oleutil.CallMethod(d, name)
	return cleanupTaskRunVariants(err, v)
}

// joinTaskRunCleanup is pure and permits fake verification that cleanup errors
// remain observable without constructing or invoking a real COM VARIANT.
func joinTaskRunCleanup(primary error, cleanups ...func() error) error {
	errs := []error{primary}
	for _, cleanup := range cleanups {
		if cleanup != nil { errs = append(errs, cleanup()) }
	}
	return errors.Join(errs...)
}

func cleanupTaskRunVariants(primary error, values ...*ole.VARIANT) error {
	cleanups := make([]func() error, 0, len(values))
	for _, value := range values {
		if value == nil { continue }
		v := value
		cleanups = append(cleanups, v.Clear)
	}
	return joinTaskRunCleanup(primary, cleanups...)
}

func takeDispatch(v *ole.VARIANT) (*ole.IDispatch, error) {
	if v == nil { return nil, errors.New("nil VARIANT") }
	if v.VT != ole.VT_DISPATCH {
		vt := v.VT
		return nil, errors.Join(fmt.Errorf("VARIANT type %d, want dispatch", vt), v.Clear())
	}
	borrowed := v.ToIDispatch()
	if borrowed == nil { return nil, errors.Join(errors.New("nil IDispatch"), v.Clear()) }
	// QueryInterface creates an independently-owned reference; clearing the
	// source VARIANT then cannot invalidate the retained interface.
	dispatch, err := borrowed.QueryInterface(ole.IID_IDispatch)
	clearErr := v.Clear()
	var clearFailure error
	if clearErr != nil { clearFailure = fmt.Errorf("clear dispatch VARIANT: %w", clearErr) }
	if err != nil {
		if dispatch != nil { dispatch.Release() }
		return nil, errors.Join(err, clearFailure)
	}
	if dispatch == nil {
		return nil, errors.Join(errors.New("QueryInterface returned nil IDispatch"), clearFailure)
	}
	if clearFailure != nil {
		dispatch.Release()
		return nil, clearFailure
	}
	return dispatch, nil
}

