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
		"format":               "mi-mcp-provider/v2",
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
	socket, ok := invocation["socket"].(map[string]any)
	if !ok {
		t.Fatalf("invocation.socket missing or malformed: %v", invocation["socket"])
	}
	for key, expected := range map[string]any{
		"transport":            "unix",
		"path_env":             "XDG_RUNTIME_DIR",
		"filename":             "mi-lsp.sock",
		"mode":                 "0600",
		"framing":              "jsonl",
		"max_request_bytes":    float64(cliSocketMaxRequest),
		"fallback":             "connection_failure_only",
		"retry_after_response": false,
	} {
		if socket[key] != expected {
			t.Errorf("invocation.socket.%s = %v, want %v", key, socket[key], expected)
		}
	}
	if actual["binary"] != "mi-lsp" || actual["description"] == "" || len(actual["tags"].([]any)) == 0 {
		t.Fatalf("v2 manifest is missing required catalog fields: %v", actual)
	}
	operations, ok := actual["operations"].([]any)
	if !ok || len(operations) != 15 {
		t.Fatalf("operations = %v, want q, intent, multi-read, wiki graph/contracts, and management operations", actual["operations"])
	}
	for _, required := range []string{"q", "intent", "multi-read", "wiki_graph", "wiki_validate_contract", "doctor", "repair"} {
		found := false
		for _, item := range operations {
			op, _ := item.(map[string]any)
			if op["name"] == required && op["description"] != "" && op["output_schema"] != nil {
				found = true
			}
		}
		if !found {
			t.Errorf("manifest operation %q is missing required v2 fields", required)
		}
	}
	operation, _ := operations[0].(map[string]any)
	if operation["name"] != "q" || operation["effect"] != "read" {
		t.Fatalf("operation name/effect = %v/%v, want q/read", operation["name"], operation["effect"])
	}
	qInvocation, ok := operation["invocation"].(map[string]any)
	if !ok || qInvocation["command"] != "mi-lsp" {
		t.Fatalf("q v2 invocation missing: %v", operation["invocation"])
	}
	qArgs, _ := qInvocation["fixed_args"].([]any)
	if !containsJSONValue(qArgs, "q") {
		t.Errorf("q invocation fixed_args must include q, got %v", qArgs)
	}
	qMapping, _ := qInvocation["input_mapping"].(map[string]any)
	qFields, _ := qMapping["fields"].(map[string]any)
	if qFields["pipeline"] == nil {
		t.Errorf("q invocation must map pipeline argv, got %v", qFields)
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

func TestProviderMaintenanceOperations(t *testing.T) {
	manifest := newProviderManifest("test")
	operations := map[string]providerOperation{}
	for _, op := range manifest.Operations {
		operations[op.Name] = op
	}
	for _, tc := range []struct{ name, effect, contract string }{
		{"doctor", "read", "mi-lsp-doctor/v0"},
		{"repair", "write", "mi-lsp-repair/v1"},
	} {
		op, ok := operations[tc.name]
		if !ok || op.Effect != tc.effect || op.Contract != tc.contract || manifest.Contracts[tc.name] != tc.contract {
			t.Fatalf("operation %s: %+v", tc.name, op)
		}
		inv := op.Invocation
		want := []string{"--format", "json", "--client-name", "mi-mcp", "--no-auto-register", tc.name}
		if inv == nil || inv.Kind != "cli" || inv.Command != "mi-lsp" || inv.Output != "json" || inv.Socket != nil || !reflect.DeepEqual(inv.FixedArgs, want) {
			t.Fatalf("%s invocation: %+v", tc.name, inv)
		}
	}
	doctor := operations["doctor"]
	if len(doctor.InputSchema["properties"].(map[string]any)) != 0 || len(doctor.Invocation.InputMapping["fields"].(map[string]any)) != 0 {
		t.Fatal("doctor must not advertise extra flags")
	}
	repair := operations["repair"]
	props := repair.InputSchema["properties"].(map[string]any)
	fields := repair.Invocation.InputMapping["fields"].(map[string]any)
	flags := map[string]string{"workspace": "workspace", "apply": "apply", "reindex": "reindex", "worker": "worker", "worker_mode": "worker-mode"}
	if len(props) != len(flags) || len(fields) != len(flags) {
		t.Fatal("repair input mapping differs from schema")
	}
	cmd, _, err := NewRootCommand().Find([]string{"repair"})
	if err != nil || cmd == nil || cmd.Name() != "repair" {
		t.Fatalf("repair command not registered: %v", err)
	}
	for field, name := range flags {
		mapping := fields[field].(map[string]any)
		if mapping["source"] != "option" || mapping["flag"] != "--"+name {
			t.Errorf("mapping %s: %v", field, mapping)
		}
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			flag = cmd.InheritedFlags().Lookup(name)
		}
		if flag == nil {
			t.Errorf("missing CLI flag --%s", name)
			continue
		}
		if field == "apply" || field == "reindex" || field == "worker" {
			schema := props[field].(map[string]any)
			if schema["type"] != "boolean" || schema["default"] != false || flag.Value.Type() != "bool" || flag.DefValue != "false" {
				t.Errorf("unsafe boolean %s: %v %s", field, schema, flag.DefValue)
			}
		}
	}
	mode := props["worker_mode"].(map[string]any)
	if mode["default"] != "auto" || !reflect.DeepEqual(mode["enum"], []string{"auto", "framework-dependent", "self-contained"}) {
		t.Fatalf("worker_mode: %v", mode)
	}
	output := repair.OutputSchema
	properties := output["properties"].(map[string]any)
	if output["type"] != "object" || properties["schema"].(map[string]any)["const"] != "mi-lsp-repair/v1" || !reflect.DeepEqual(output["required"], []string{"schema", "ok", "mode", "runtime", "actions", "reasonCodes", "changed"}) {
		t.Fatalf("repair output schema: %v", output)
	}
	for _, name := range []string{"actions", "reasonCodes"} {
		if properties[name].(map[string]any)["type"] != "array" {
			t.Errorf("%s must be an array", name)
		}
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
