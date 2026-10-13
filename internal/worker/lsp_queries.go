package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// Legacy inferred offsets are 1-based byte columns. LSP positions are UTF-16.
func utf16ColumnForByteOffset(file string, line, column int) (int, error) {
	data, err := readKotlinMetadata(file)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return 0, errors.New("invalid line")
	}
	text := lines[line-1]
	offset := column - 1
	if offset < 0 || offset > len(text) || !utf8.ValidString(text[:offset]) {
		return 0, errors.New("invalid byte column")
	}
	return len(utf16.Encode([]rune(text[:offset]))) + 1, nil
}

func validateUTF16Position(file string, line, column int) error {
	data, err := readKotlinMetadata(file)
	if err != nil {
		return err
	}
	if len(data) > maxLSPDocumentBytes {
		return errors.New("document exceeds bound")
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) || column < 1 {
		return errors.New("invalid UTF-16 position")
	}
	units := 0
	for _, r := range strings.TrimSuffix(lines[line-1], "\r") {
		if units == column-1 {
			return nil
		}
		width := 1
		if r > 0xffff {
			width = 2
		}
		units += width
		if units > column-1 {
			return errors.New("column splits UTF-16 surrogate pair")
		}
	}
	if units != column-1 {
		return errors.New("UTF-16 column exceeds line")
	}
	return nil
}

func (c *LSPClient) definition(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	file, line, col, _, err := c.resolveLocation(request)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	if err = c.openDocument(file); err != nil {
		return model.WorkerResponse{}, err
	}
	raw, err := c.sendSemanticRequest("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": buildFileURI(file)}, "position": lspPositionMap(line, col)})
	if err != nil {
		return model.WorkerResponse{}, err
	}
	locations := []lspLocation{}
	if string(raw) != "null" && len(raw) > 0 {
		if err = json.Unmarshal(raw, &locations); err != nil {
			var location lspLocation
			if err = json.Unmarshal(raw, &location); err != nil {
				return model.WorkerResponse{}, err
			}
			locations = []lspLocation{location}
		}
	}
	items := make([]map[string]any, 0, len(locations))
	for _, location := range locations {
		if !strings.HasPrefix(location.URI, "file://") {
			return model.WorkerResponse{}, errors.New("unsupported definition URI")
		}
		items = append(items, map[string]any{"file": uriToPath(location.URI), "line": location.Range.Start.Line + 1, "column": location.Range.Start.Character + 1, "position_encoding": "utf-16"})
	}
	return model.WorkerResponse{Ok: true, Backend: c.backendName(), Items: items, Stats: model.Stats{Symbols: len(items)}}, nil
}

// Document symbols remain PSI/extracted structure, never compiler identities.
func (c *LSPClient) documentSymbols(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	file, _ := request.Payload["file"].(string)
	if file == "" {
		return model.WorkerResponse{}, errors.New("explicit file required")
	}
	copy := request
	copy.Payload = map[string]any{"file": file, "line": 1, "column": 1}
	file, _, _, _, err := c.resolveLocation(copy)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	if err = c.openDocument(file); err != nil {
		return model.WorkerResponse{}, err
	}
	raw, err := c.sendSemanticRequest("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": buildFileURI(file)}})
	if err != nil {
		return model.WorkerResponse{}, err
	}
	var symbols []map[string]any
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &symbols); err != nil {
			return model.WorkerResponse{}, err
		}
	}
	items := make([]map[string]any, 0, len(symbols))
	for _, symbol := range symbols {
		items = append(items, map[string]any{"file": file, "symbol": symbol, "origin": "extracted", "identity": "structural", "position_encoding": "utf-16"})
	}
	return model.WorkerResponse{Ok: true, Backend: c.backendName(), Items: items, Stats: model.Stats{Symbols: len(items)}}, nil
}
