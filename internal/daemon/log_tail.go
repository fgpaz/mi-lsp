package daemon

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
)

type LogTailLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

func ReadLogTailFile(path string, tail int, maxBytes int64) ([]LogTailLine, bool, error) {
	if tail <= 0 {
		tail = 50
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	lineCount, err := countFileLines(path)
	if err != nil {
		return nil, false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	readSize := info.Size()
	truncated := false
	if readSize > maxBytes {
		readSize = maxBytes
		truncated = true
	}
	if _, err := file.Seek(info.Size()-readSize, io.SeekStart); err != nil {
		return nil, false, err
	}
	data := make([]byte, int(readSize))
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, false, err
	}
	if truncated {
		if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
			data = data[idx+1:]
		}
	}
	content := strings.TrimRight(string(data), "\r\n")
	if strings.TrimSpace(content) == "" {
		return []LogTailLine{}, truncated, nil
	}
	rows := strings.Split(content, "\n")
	if tail > len(rows) {
		tail = len(rows)
	}
	start := len(rows) - tail
	firstLine := lineCount - len(rows) + start + 1
	if firstLine < 1 {
		firstLine = 1
	}
	items := make([]LogTailLine, 0, tail)
	for idx, line := range rows[start:] {
		items = append(items, LogTailLine{Line: firstLine + idx, Text: strings.TrimRight(line, "\r")})
	}
	return items, truncated, nil
}

func countFileLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}
