package milx

import (
	"encoding/binary"
	"fmt"
	"io"
)

func EncodeFrame(payload []byte) ([]byte, error) {
	if len(payload) > MaxFrameBytes {
		return nil, &MILXError{Code: "GPH_MILX_FRAME_INVALID", Stage: "frame", SanitizedSummary: "frame exceeds maximum size"}
	}
	out := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(out, uint32(len(payload)))
	copy(out[4:], payload)
	return out, nil
}
func WriteFrame(w io.Writer, payload []byte) error {
	b, err := EncodeFrame(payload)
	if err != nil {
		return err
	}
	n, err := w.Write(b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return io.ErrShortWrite
	}
	return nil
}
func ReadFrame(r io.Reader) ([]byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(h[:])
	if n > MaxFrameBytes {
		return nil, &MILXError{Code: "GPH_MILX_FRAME_INVALID", Stage: "frame", SanitizedSummary: "frame exceeds maximum size"}
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("partial frame: %w", err)
	}
	if len(b) > 0 && b[0] == 0xef && len(b) >= 3 && b[1] == 0xbb && b[2] == 0xbf {
		return nil, fmt.Errorf("BOM is forbidden")
	}
	return b, nil
}
