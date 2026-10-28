//go:build windows

package winhost

import (
 "bytes"
 "errors"
 "testing"
 "unsafe"
)

type faultWriter struct { bytes.Buffer; writeErr, closeErr error; short bool; closed bool }
func (w *faultWriter) Write(p []byte) (int,error) { if w.writeErr != nil { return 0,w.writeErr }; if w.short { return w.Buffer.Write(p[:len(p)-1]) }; return w.Buffer.Write(p) }
func (w *faultWriter) Close() error { w.closed=true; return w.closeErr }
func TestWriteResultTransportCollision(t *testing.T) {
 root:=uint32(125)
 result:=outcome{Protocol:"execsafe-host/v1", Kind:"root_exit", ExitCode:root, RootExitCode:&root}
 cases:=[]struct{name string; w *faultWriter; want uint32}{
  {"complete", &faultWriter{},0}, {"write", &faultWriter{writeErr:errors.New("write")},125},
  {"short", &faultWriter{short:true},125}, {"close", &faultWriter{closeErr:errors.New("close")},125},
 }
 for _,tc:=range cases { t.Run(tc.name,func(t *testing.T) { if got:=writeResult(tc.w,result);got!=tc.want {t.Fatalf("transport %d",got)}; if !tc.w.closed {t.Fatal("not closed")}; if tc.name=="complete" || tc.name=="close" { if !bytes.Contains(tc.w.Bytes(),[]byte("execsafe-host/v1 root_exit 125\n")) || !bytes.Contains(tc.w.Bytes(),[]byte(`"root_exit_code":125`)) {t.Fatalf("record %q",tc.w.Bytes())} } }) }
}
func TestWindows64Layouts(t *testing.T) {
 if unsafe.Sizeof(uintptr(0)) != 8 { t.Skip("64-bit layout") }
 if got:=unsafe.Sizeof(jobBasicLimit{}); got!=64 {t.Errorf("basic %d",got)}
 if got:=unsafe.Sizeof(jobExtendedLimit{}); got!=144 {t.Errorf("extended %d",got)}
 if got:=unsafe.Sizeof(startupInfoEx{}); got!=112 {t.Errorf("startupEx %d",got)}
 x:=jobExtendedLimit{}; x.basic.flags=0x2000; if x.basic.flags!=0x2000 {t.Fatal("kill on close flag")}
}
