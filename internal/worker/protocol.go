package worker

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// MaxFrameSize is the maximum allowed frame size (256MB).
// Frames exceeding this size are rejected to prevent OOM DoS attacks.
const MaxFrameSize = 256 << 20

func WriteFrame(writer io.Writer, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(body)))
	if _, err := writer.Write(header); err != nil {
		return err
	}
	_, err = writer.Write(body)
	return err
}

func ReadFrame(reader io.Reader, payload any) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	length := binary.BigEndian.Uint32(header)
	// SEC-01: Validate frame size before allocating memory
	if int64(length) > MaxFrameSize {
		return fmt.Errorf("frame too large: %d > %d", length, MaxFrameSize)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return err
	}
	return json.Unmarshal(body, payload)
}
