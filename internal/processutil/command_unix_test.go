//go:build !windows

package processutil

import (
	"context"
	"testing"
)

func TestUnixConstructorsLeaveProcessAttributesUnchanged(t *testing.T) {
	a := Command("example")
	b := CommandContext(context.Background(), "example")
	if a.SysProcAttr != nil || b.SysProcAttr != nil {
		t.Fatalf("unexpected attributes: %+v, %+v", a.SysProcAttr, b.SysProcAttr)
	}
	ConfigureDetachedCommand(a)
	if a.SysProcAttr == nil || !a.SysProcAttr.Setpgid {
		t.Fatalf("detached attributes: %+v", a.SysProcAttr)
	}
}
