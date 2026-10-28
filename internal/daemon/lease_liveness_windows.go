//go:build windows

package daemon

import (
 "errors"
 "syscall"
 "golang.org/x/sys/windows"
)

func probeLeasePID(pid int) string {
 if pid<=0 { return "indeterminate" }
 h,err:=syscall.OpenProcess(processQueryLimitedInformation,false,uint32(pid))
 if err!=nil { if errors.Is(err,windows.ERROR_INVALID_PARAMETER) {return "dead"}; return "indeterminate" }
 defer syscall.CloseHandle(h)
 var code uint32
 if err:=syscall.GetExitCodeProcess(h,&code); err!=nil {return "indeterminate"}
 if code==stillActive {return "alive"}; return "dead"
}
