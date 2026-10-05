package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestProviderManifestCommandPrintsContractAndRealQFlags(t *testing.T) {
	root := NewRootCommand()
	command, _, err := root.Find([]string{"provider-manifest"})
	if err != nil {
		t.Fatal(err)
	}
	if command == nil {
		t.Fatal("provider-manifest command is not registered")
	}

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"provider-manifest", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	var actual map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &actual); err != nil {
		t.Fatalf("command output is not clean JSON: %v\n%s", err, stdout.String())
	}
	for key, expected := range map[string]any{
		"format":               "mi-mcp-provider/v1",
		"namespace":            "milsp",
		"min_provider_version": "0.10.0",
		"auth":                 "none",
	} {
		if actual[key] != expected {
			t.Errorf("%s = %v, want %v", key, actual[key], expected)
		}
	}
	version, _ := actual["provider_version"].(string)
	if version == "" || strings.HasPrefix(version, "v") {
		t.Errorf("provider_version must be the --version semver without v, got %q", version)
	}

	contracts, ok := actual["contracts"].(map[string]any)
	if !ok || contracts["q"] != "q-v1" {
		t.Errorf("contracts.q = %v, want q-v1", actual["contracts"])
	}
	invocation, ok := actual["invocation"].(map[string]any)
	if !ok {
		t.Fatalf("invocation missing or malformed: %v", actual["invocation"])
	}
	fixedArgs, _ := invocation["fixed_args"].([]any)
	if !containsJSONValue(fixedArgs, "--no-auto-register") {
		t.Errorf("invocation.fixed_args must include --no-auto-register, got %v", fixedArgs)
	}
	operations, ok := actual["operations"].([]any)
	if !ok || len(operations) != 1 {
		t.Fatalf("operations = %v, want one q operation", actual["operations"])
	}
	operation, _ := operations[0].(map[string]any)
	if operation["name"] != "q" || operation["effect"] != "read" {
		t.Fatalf("operation name/effect = %v/%v, want q/read", operation["name"], operation["effect"])
	}
	schema, ok := operation["input_schema"].(map[string]any)
	if !ok {
		t.Fatalf("q input_schema missing or malformed: %v", operation["input_schema"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("input_schema.properties missing or malformed: %v", schema["properties"])
	}

	qCommand, _, err := NewRootCommand().Find([]string{"q"})
	if err != nil || qCommand == nil {
		t.Fatalf("find q command: command=%v err=%v", qCommand, err)
	}
	propertyFlags := map[string]string{
		"workspace":  "workspace",
		"budget":     "budget",
		"max_bytes":  "max-bytes",
		"timeout_ms": "timeout-ms",
		"session_id": "session-id",
		"page":       "page",
		"fresh":      "fresh",
		"dedupe":     "dedupe",
	}
	if len(properties) != len(propertyFlags)+1 || properties["pipeline"] == nil {
		t.Errorf("schema properties should contain pipeline and exactly the real q options; got %v", properties)
	}
	for property, flagName := range propertyFlags {
		if _, ok := properties[property]; !ok {
			t.Errorf("schema property %q missing", property)
			continue
		}
		flag := qCommand.Flags().Lookup(flagName)
		if flag == nil {
			flag = qCommand.InheritedFlags().Lookup(flagName)
		}
		if flag == nil {
			t.Errorf("input_schema property %q refers to absent q flag --%s", property, flagName)
		}
	}

	manifestBytes, err := os.ReadFile("../../integrations/mi-mcp/provider-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var checkedIn map[string]any
	if err := json.Unmarshal(manifestBytes, &checkedIn); err != nil {
		t.Fatalf("checked-in manifest is invalid JSON: %v", err)
	}
	checkedIn["provider_version"] = actual["provider_version"]
	if !reflect.DeepEqual(actual, checkedIn) {
		t.Errorf("checked-in manifest differs from command output (after normalizing provider_version)\ncommand: %#v\nfile: %#v", actual, checkedIn)
	}
}

func containsJSONValue(values []any, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
