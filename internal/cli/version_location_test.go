package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionRuntimeCapability(t *testing.T) {
	info := buildVersionInfo("")
	if info.Runtime.ExecutablePath == "" || len(info.Capabilities) != 1 || info.Capabilities[0] != "repair-v1" {
		t.Fatalf("runtime=%+v capabilities=%v", info.Runtime, info.Capabilities)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"runtime"`, `"executablePath"`, `"managedRoot"`, `"managedExecutablePath"`, `"receiptPath"`, `"capabilities"`, `"repair-v1"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("missing %s in %s", field, data)
		}
	}
}
