package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWindowsVersionResourceTemplate(t *testing.T) {
	data, err := os.ReadFile("winres.json")
	if err != nil {
		t.Fatal(err)
	}
	var resources struct {
		Version map[string]map[string]struct {
			Fixed map[string]string            `json:"fixed"`
			Info  map[string]map[string]string `json:"info"`
		} `json:"RT_VERSION"`
	}
	if err := json.Unmarshal(data, &resources); err != nil {
		t.Fatal(err)
	}
	v := resources.Version["#1"]["0000"]
	info := v.Info["0409"]
	if info["CompanyName"] != "mi-lsp contributors" || info["ProductName"] != "mi-lsp" || info["FileVersion"] != "0.0.0.0" || v.Fixed["file_version"] != "0.0.0.0" {
		t.Fatalf("unexpected version resource: fixed=%v info=%v", v.Fixed, info)
	}
}
