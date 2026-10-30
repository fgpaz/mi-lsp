package model

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/token"
	"io"
	"path"
	"path/filepath"
	"strings"
)

const (
	NativeSymbolSelectorPrefix    = "mi-lsp-symbol:v1:"
	NativeSymbolSelectorMaxLength = 4096
	NativeSymbolSelectorMaxBytes  = 3072
)

// NativeSymbolSelector is an untrusted, versioned target selector. It is not
// proof of authority; callers must revalidate identity, path, snapshot, and
// semantic readiness before performing an operation.
type NativeSymbolSelector struct {
	Version       int    `json:"v"`
	Workspace     string `json:"w"`
	WorkspaceRoot string `json:"wr"`
	RepoID        string `json:"ri"`
	RepoRoot      string `json:"rr"`
	File          string `json:"f"`
	Line          int    `json:"l"`
	Column        int    `json:"c"`
	Name          string `json:"n"`
	SHA256        string `json:"h"`
}

func EncodeNativeSymbolSelector(selector NativeSymbolSelector) (string, error) {
	if err := validateNativeSymbolSelector(selector); err != nil {
		return "", err
	}
	payload, err := json.Marshal(selector)
	if err != nil || len(payload) > NativeSymbolSelectorMaxBytes {
		return "", errors.New("symbol selector payload exceeds limit")
	}
	encoded := NativeSymbolSelectorPrefix + base64.RawURLEncoding.EncodeToString(payload)
	if len(encoded) > NativeSymbolSelectorMaxLength {
		return "", errors.New("symbol selector exceeds limit")
	}
	return encoded, nil
}

// DecodeNativeSymbolSelector returns recognized=false only for a bare symbol.
// Values using the selector namespace but an unknown version fail closed.
func DecodeNativeSymbolSelector(value string) (selector NativeSymbolSelector, recognized bool, err error) {
	if !strings.HasPrefix(value, "mi-lsp-symbol:") {
		return NativeSymbolSelector{}, false, nil
	}
	if !strings.HasPrefix(value, NativeSymbolSelectorPrefix) || len(value) > NativeSymbolSelectorMaxLength {
		return NativeSymbolSelector{}, true, errors.New("unsupported or oversized symbol selector")
	}
	encoded := strings.TrimPrefix(value, NativeSymbolSelectorPrefix)
	payload, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(payload) == 0 || len(payload) > NativeSymbolSelectorMaxBytes {
		return NativeSymbolSelector{}, true, errors.New("invalid symbol selector encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&selector); err != nil {
		return NativeSymbolSelector{}, true, errors.New("invalid symbol selector payload")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return NativeSymbolSelector{}, true, errors.New("symbol selector payload has trailing data")
	}
	if err := validateNativeSymbolSelector(selector); err != nil {
		return NativeSymbolSelector{}, true, err
	}
	canonical, err := json.Marshal(selector)
	if err != nil || !bytes.Equal(payload, canonical) {
		return NativeSymbolSelector{}, true, errors.New("symbol selector payload is not canonical")
	}
	return selector, true, nil
}

func validateNativeSymbolSelector(selector NativeSymbolSelector) error {
	if selector.Version != 1 || selector.Workspace == "" || selector.RepoID == "" || selector.Name == "" || len(selector.Name) > 256 || !token.IsIdentifier(selector.Name) {
		return errors.New("symbol selector identity or name is invalid")
	}
	if len(selector.Workspace) > 256 || len(selector.RepoID) > 256 || !filepath.IsAbs(selector.WorkspaceRoot) || !filepath.IsAbs(selector.RepoRoot) || filepath.Clean(selector.WorkspaceRoot) != selector.WorkspaceRoot || filepath.Clean(selector.RepoRoot) != selector.RepoRoot {
		return errors.New("symbol selector physical identity is invalid")
	}
	if len(selector.WorkspaceRoot) > 1024 || len(selector.RepoRoot) > 1024 || selector.Line < 1 || selector.Line > 1<<24 || selector.Column < 1 || selector.Column > 1<<20 {
		return errors.New("symbol selector bounds are invalid")
	}
	if selector.File == "" || len(selector.File) > 1024 || strings.ContainsAny(selector.File, "\\\x00:") || path.IsAbs(selector.File) || path.Clean(selector.File) != selector.File || selector.File == "." || selector.File == ".." || strings.HasPrefix(selector.File, "../") {
		return errors.New("symbol selector relative file is invalid")
	}
	for _, segment := range strings.Split(selector.File, "/") {
		if segment == ".git" || segment == ".." || segment == "" {
			return errors.New("symbol selector file is outside the allowed source tree")
		}
	}
	digest, err := hex.DecodeString(selector.SHA256)
	if err != nil || len(digest) != 32 || strings.ToLower(selector.SHA256) != selector.SHA256 {
		return errors.New("symbol selector snapshot digest is invalid")
	}
	return nil
}
