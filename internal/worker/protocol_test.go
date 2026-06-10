package worker

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestReadFrameRejectsOversizedFrames verifies SEC-01: frame size validation.
func TestReadFrameRejectsOversizedFrames(t *testing.T) {
	// Create a frame header with a size exceeding MaxFrameSize
	buf := new(bytes.Buffer)
	oversizedLen := uint32(MaxFrameSize + 1)
	binary.Write(buf, binary.BigEndian, oversizedLen)

	var payload interface{}
	err := ReadFrame(buf, &payload)
	if err == nil {
		t.Fatal("ReadFrame with oversized length should error, got nil")
	}
	if err.Error() != "frame too large: 268435457 > 268435456" {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestReadFrameAcceptsValidFrames verifies that correctly sized frames are accepted.
func TestReadFrameAcceptsValidFrames(t *testing.T) {
	testData := map[string]interface{}{"test": "data"}
	buf := new(bytes.Buffer)

	// Write a valid frame
	if err := WriteFrame(buf, testData); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	// Read it back
	var result map[string]interface{}
	if err := ReadFrame(buf, &result); err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if result["test"] != "data" {
		t.Fatalf("payload mismatch: got %v", result)
	}
}

// TestReadFrameWithMaxSize verifies that a frame at exactly MaxFrameSize is accepted.
func TestReadFrameWithMaxSize(t *testing.T) {
	buf := new(bytes.Buffer)
	// Write a header with MaxFrameSize
	binary.Write(buf, binary.BigEndian, uint32(MaxFrameSize))
	// Write exactly MaxFrameSize bytes of padding
	padding := make([]byte, MaxFrameSize)
	buf.Write(padding)

	// Reset to beginning
	readBuf := bytes.NewReader(buf.Bytes())
	var payload []byte
	err := ReadFrame(readBuf, &payload)
	// This will fail on JSON unmarshal but should not fail on size validation
	if err != nil && err.Error() == "frame too large" {
		t.Fatal("ReadFrame should accept frames at exactly MaxFrameSize")
	}
}
