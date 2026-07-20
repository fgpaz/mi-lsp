package milx

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"strings"
	"testing"
)

func testManifest() Manifest {
	return Manifest{Schema: ManifestSchema, ExtensionID: "demo.ext", ExtensionVersion: "1.0.0", ExecutableSHA256: strings.Repeat("a", 64), ProtocolMin: 1, ProtocolMax: 1, Operations: []string{"describe", "execute"}, InputSchemas: []string{"milx-pack/v1"}, OutputSchemas: []string{"milx-result/v1"}, Capabilities: []string{"analysis.emit"}, Deterministic: true}
}

func TestManifestValidationDigestAndExecutableDigest(t *testing.T) {
	m := testManifest()
	d, err := m.Digest()
	if err != nil || hex.EncodeToString(d[:]) == "" {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.Capabilities = []string{"network"} },
		func(m *Manifest) { m.Operations = []string{"unknown"} },
		func(m *Manifest) { m.InputSchemas = []string{"unknown/v1"} },
		func(m *Manifest) { m.ExecutableSHA256 = strings.Repeat("A", 64) },
		func(m *Manifest) { m.Operations = []string{"execute", "describe"} },
	} {
		bad := testManifest()
		mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	payload := []byte("executable")
	if err := VerifyExecutableDigest(payload, DigestHex(payload)); err != nil {
		t.Fatal(err)
	}
	if err := VerifyExecutableDigest(payload, strings.Repeat("0", 64)); err == nil || err.(*MILXError).Code != "GPH_MILX_EXECUTABLE_DIGEST_MISMATCH" {
		t.Fatal("digest mismatch accepted")
	}
}

func TestCanonicalJSONGoldensAndRejectedValues(t *testing.T) {
	v := map[string]any{"z": 1, "a": map[string]any{"d": 2, "c": 1}}
	want := `{"a":{"c":1,"d":2},"z":1}`
	for i := 0; i < 30; i++ {
		b, err := CanonicalJSON(v)
		if err != nil || string(b) != want {
			t.Fatalf("golden %d: %s %v", i, b, err)
		}
	}
	for _, v := range []any{
		map[string]any{"network": true}, map[string]any{"NeTwOrK": true},
		map[string]any{"nested": map[string]any{"SeCrEt": true}},
		map[string]any{"n": 1.5}, map[string]any{"n": math.NaN()}, map[string]any{"n": math.Inf(1)},
	} {
		if _, err := CanonicalJSON(v); err == nil {
			t.Fatal("forbidden value accepted")
		}
	}
}

func TestDecodeCanonicalRejectsNonCanonicalAndInvalidUTF8(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("{\"z\":1,\"a\":2}"), []byte("{\"a\":1 }"), []byte("{\"n\":1.0}"),
		[]byte("\xef\xbb\xbf{}"), []byte{'{', 0xff, '}'}, []byte("{} trailing"),
	} {
		var out any
		if err := DecodeCanonical(data, &out); err == nil {
			t.Fatalf("invalid canonical data accepted: %q", data)
		}
	}
	var out map[string]any
	if err := DecodeCanonical([]byte(`{"a":1,"z":2}`), &out); err != nil || out["a"] != float64(1) {
		t.Fatal(err, out)
	}
}

func TestFramesBoundariesPartialBOMAndNextFrame(t *testing.T) {
	max := make([]byte, MaxFrameBytes)
	if b, err := EncodeFrame(max); err != nil || len(b) != MaxFrameBytes+4 {
		t.Fatal(err)
	}
	if _, err := EncodeFrame(make([]byte, MaxFrameBytes+1)); err == nil {
		t.Fatal("oversize accepted")
	}
	a, _ := EncodeFrame([]byte("one"))
	b, _ := EncodeFrame([]byte("two"))
	r := bytes.NewReader(append(a, b...))
	for _, want := range []string{"one", "two"} {
		got, err := ReadFrame(r)
		if err != nil || string(got) != want {
			t.Fatal(got, err)
		}
	}
	if _, err := ReadFrame(bytes.NewReader([]byte{0, 0, 0, 4, 'x'})); err == nil {
		t.Fatal("partial accepted")
	}
	var oversized [4]byte
	binary.BigEndian.PutUint32(oversized[:], MaxFrameBytes+1)
	if _, err := ReadFrame(bytes.NewReader(oversized[:])); err == nil {
		t.Fatal("uint32 oversize accepted")
	}
	bom, _ := EncodeFrame([]byte("\xef\xbb\xbf{}"))
	if _, err := ReadFrame(bytes.NewReader(bom)); err == nil {
		t.Fatal("BOM frame accepted")
	}
	if _, err := ReadFrame(io.LimitReader(strings.NewReader(""), 0)); err == nil {
		t.Fatal("empty frame accepted")
	}
}

func TestEnvelopeRequiredLimitsStatusesAndOperationPayloads(t *testing.T) {
	valid := Envelope{Schema: EnvelopeSchema, RequestID: "r", Operation: "execute", ProtocolVersion: ProtocolVersion, Payload: []byte(`{}`)}
	if err := ValidateEnvelope(valid, false); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Envelope{
		{}, {Schema: EnvelopeSchema, RequestID: "r", Operation: "unknown", ProtocolVersion: 1, Payload: []byte(`{}`)},
		{Schema: EnvelopeSchema, RequestID: strings.Repeat("r", MaxRequestID+1), Operation: "execute", ProtocolVersion: 1, Payload: []byte(`{}`)},
		{Schema: EnvelopeSchema, RequestID: "r", Operation: "execute", ProtocolVersion: 2, Payload: []byte(`{}`)},
	} {
		if err := ValidateEnvelope(bad, false); err == nil {
			t.Fatal("invalid request envelope accepted")
		}
	}
	for _, status := range []string{"ok", "rejected", "canceled", "timeout", "failed"} {
		response := valid
		response.Status = status
		if err := ValidateEnvelope(response, true); err != nil {
			t.Fatal(status, err)
		}
	}
	response := valid
	response.Status = "invalid"
	if err := ValidateEnvelope(response, true); err == nil {
		t.Fatal("invalid response status accepted")
	}
	for _, operation := range []string{"describe", "prepare", "execute", "cancel", "health", "shutdown"} {
		payload := []byte(`{"x":1}`)
		if operation == "describe" || operation == "health" || operation == "shutdown" {
			payload = []byte(`{}`)
		}
		if err := ValidateOperationPayload(operation, payload); err != nil {
			t.Fatal(operation, err)
		}
	}
	for _, tc := range []struct {
		operation string
		payload   []byte
	}{{"describe", []byte(`{"x":1}`)}, {"execute", []byte("null")}, {"cancel", nil}, {"unknown", []byte(`{}`)}, {"prepare", []byte(`{ "a":1}`)}} {
		if err := ValidateOperationPayload(tc.operation, tc.payload); err == nil {
			t.Fatal("invalid operation payload accepted")
		}
	}
}

func TestTransitions(t *testing.T) {
	if !ValidTransition(StatePrepared, "execute") || ValidTransition(StateSpawned, "execute") || ValidTransition(StateShutdown, "health") {
		t.Fatal("bad transitions")
	}
}
