package worker

import (
	"encoding/binary"
	"encoding/json"
	"io"
)

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
	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return err
	}
	return json.Unmarshal(body, payload)
}
