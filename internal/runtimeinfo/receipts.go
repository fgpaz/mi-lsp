package runtimeinfo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const receiptSchema = "mi-lsp-installations/v1"

type Installation struct {
	Component   string    `json:"component"`
	Path        string    `json:"path"`
	InstalledAt time.Time `json:"installedAt"`
}
type receipts struct {
	Schema        string         `json:"schema"`
	Installations []Installation `json:"installations"`
}

func readReceipts(path string) (receipts, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return receipts{Schema: receiptSchema, Installations: []Installation{}}, nil
	}
	if err != nil {
		return receipts{}, err
	}
	var result receipts
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return receipts{}, fmt.Errorf("invalid installation receipts: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return receipts{}, errors.New("invalid installation receipts: trailing data")
	}
	if result.Schema != receiptSchema || result.Installations == nil {
		return receipts{}, errors.New("invalid installation receipts: schema or installations")
	}
	seen := map[string]bool{}
	for _, item := range result.Installations {
		_, offset := item.InstalledAt.Zone()
		if (item.Component != "runtime" && item.Component != "worker") || seen[item.Component] || !filepath.IsAbs(item.Path) || item.InstalledAt.IsZero() || offset != 0 {
			return receipts{}, errors.New("invalid installation receipt")
		}
		seen[item.Component] = true
	}
	return result, nil
}

func Record(component, path string) error {
	if component != "runtime" && component != "worker" {
		return errors.New("invalid installation component")
	}
	if !filepath.IsAbs(path) {
		return errors.New("installation path must be absolute")
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("installation path: %w", err)
	}
	receiptPath := Locate().ReceiptPath
	if receiptPath == "" {
		return errors.New("home directory unavailable")
	}
	return WithFileLock(receiptPath, func() error {
		r, err := readReceipts(receiptPath)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("installation path: %w", err)
		}
		item := Installation{Component: component, Path: path, InstalledAt: time.Now().UTC()}
		for i := range r.Installations {
			if r.Installations[i].Component == component {
				r.Installations[i] = item
				return WriteJSONAtomic(receiptPath, r)
			}
		}
		r.Installations = append(r.Installations, item)
		return WriteJSONAtomic(receiptPath, r)
	})
}

func Removed() ([]Installation, error) {
	path := Locate().ReceiptPath
	if path == "" {
		return nil, errors.New("home directory unavailable")
	}
	r, err := readReceipts(path)
	if err != nil {
		return nil, err
	}
	removed := []Installation{}
	for _, item := range r.Installations {
		_, err := os.Stat(item.Path)
		if errors.Is(err, os.ErrNotExist) {
			removed = append(removed, item)
		} else if err != nil {
			return nil, err
		}
	}
	return removed, nil
}
