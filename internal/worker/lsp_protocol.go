package worker

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// writeLSPMessage writes a JSON-RPC message with Content-Length header.
func writeLSPMessage(writer io.Writer, msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if len(body) > 8<<20 {
		return errors.New("LSP message exceeds 8 MiB")
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := writer.Write([]byte(header)); err != nil {
		return err
	}
	_, err = writer.Write(body)
	return err
}

// readLSPMessage reads a JSON-RPC message from a Content-Length-framed stream.
func readLSPMessage(reader *bufio.Reader) (json.RawMessage, error) {
	contentLength := 0
	headerBytes := 0
	seenLength := false
	for {
		fragment, err := reader.ReadSlice('\n')
		line := string(fragment)
		headerBytes += len(fragment)
		if headerBytes > 8192 {
			return nil, errors.New("LSP headers exceed limit")
		}
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			if seenLength {
				return nil, errors.New("duplicate Content-Length")
			}
			seenLength = true
			value := strings.TrimSpace(line[len("Content-Length:"):])
			contentLength, err = strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
		}
	}
	if contentLength <= 0 || contentLength > 8<<20 {
		return nil, errors.New("LSP message has invalid content length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// buildFileURI converts an absolute file path to a file:// URI.
func buildFileURI(absPath string) string {
	absPath = filepath.ToSlash(absPath)
	if runtime.GOOS == "windows" && len(absPath) >= 2 && absPath[1] == ':' {
		absPath = "/" + absPath
	}
	return (&url.URL{Scheme: "file", Path: absPath}).String()
}

// uriToPath converts a file:// URI back to a local absolute path, decoding
// percent-escapes. On Windows the leading slash of /C:/x is dropped.
func uriToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return filepath.FromSlash(uri)
	}
	path := ""
	if parsed, err := url.Parse(uri); err == nil {
		path = parsed.Path
		if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
			path = "/" + parsed.Host + path
		}
	} else {
		path = strings.TrimPrefix(uri, "file://")
	}
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

// lspPositionMap converts 1-based line/column to LSP 0-based position map.
func lspPositionMap(line, col int) map[string]int {
	l := line - 1
	c := col - 1
	if l < 0 {
		l = 0
	}
	if c < 0 {
		c = 0
	}
	return map[string]int{"line": l, "character": c}
}
