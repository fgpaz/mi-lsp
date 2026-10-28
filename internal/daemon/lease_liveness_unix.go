//go:build !windows

package daemon

import (
 "errors"
 "syscall"
)

func probeLeasePID(pid int) string {
 if pid<=0 { return "indeterminate" }
 err:=syscall.Kill(pid,0)
 if err==nil { return "alive" }
 if errors.Is(err,syscall.ESRCH) { return "dead" }
 return "indeterminate"
}
