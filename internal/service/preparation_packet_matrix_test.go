package service

import (
	"encoding/json"
	"github.com/fgpaz/mi-lsp/internal/model"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func packetMatrixFixture(t *testing.T) (*App, string, string, model.PreparationPacket) {
	t.Helper()
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, ".docs/wiki/00_gobierno_documental.md", "governance")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "generation = 1")
	p := model.PreparationPacket{Schema: model.PreparationSchema, Workspace: model.PreparationWorkspace{CanonicalRoot: root, IdentityDigest: preparationDigest(root)}, Task: model.PreparationTask{Digest: preparationDigest("task"), Intent: "intent"}, Scope: model.PreparationScope{AllowedPaths: []string{"src/Hello.cs"}, DeniedClasses: []string{"authorization"}, ReadOnly: true}, Lineage: model.PreparationLineage{PreparationID: "id", CreatedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}, Evidence: model.PreparationEvidence{Root: root}, Status: "ready", Compatibility: "current"}
	p.Semantic.GovernanceDigest = preparationGovernanceDigest(root)
	p.Semantic.IndexDigest = preparationIndexGeneration(root)
	p.PacketDigest = packetDigest(p)
	return New(root, nil), root, name, p
}
func matrixVerify(t *testing.T, app *App, root, name string, p model.PreparationPacket, payload map[string]any) model.PreparationResult {
	t.Helper()
	if p.Schema == "" {
		env, _ := app.verifyPreparation(root, model.CommandRequest{Context: model.QueryOptions{Workspace: name}, Payload: payload})
		return env.Items.([]model.PreparationResult)[0]
	}
	path := filepath.Join(root, "packet.json")
	b, _ := json.Marshal(p)
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["packet_path"]; !ok {
		payload["packet_path"] = path
	}
	if _, ok := payload["evidence_root"]; !ok {
		payload["evidence_root"] = root
	}
	env, err := app.verifyPreparation(root, model.CommandRequest{Context: model.QueryOptions{Workspace: name}, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return env.Items.([]model.PreparationResult)[0]
}
func TestPreparationPacketCoreTP01SeparateChildProcess(t *testing.T) {
	const childEnv = "MI_LSP_PREPARATION_TP01_CHILD"
	if os.Getenv(childEnv) == "1" {
		root := os.Getenv("MI_LSP_PREPARATION_TP01_ROOT")
		packetPath := os.Getenv("MI_LSP_PREPARATION_TP01_PACKET")
		if root == "" || packetPath == "" {
			t.Fatal("child preparation fixture is incomplete")
		}
		app := New(root, nil)
		env, err := app.verifyPreparation(root, model.CommandRequest{
			Context: model.QueryOptions{Workspace: "neutral-child"},
			Payload: map[string]any{
				"packet_path":   packetPath,
				"evidence_root": root,
				"task":          "task",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		results := env.Items.([]model.PreparationResult)
		if len(results) != 1 || results[0].Code != "PREPARATION_READY" || !results[0].EvidenceOnly || !results[0].Transferable {
			t.Fatalf("child verification result = %+v", results)
		}
		return
	}

	app, root, name, _ := packetMatrixFixture(t)
	packetPath := filepath.Join(root, "portable-preparation.json")
	env, err := app.Execute(t.Context(), model.CommandRequest{
		Operation: "prepare.create",
		Context:   model.QueryOptions{Workspace: name},
		Payload: map[string]any{
			"task":           "task",
			"intent":         "read_only",
			"output":         packetPath,
			"evidence_root":  root,
			"affected_paths": []string{"src/Hello.cs"},
		},
	})
	if err != nil || !env.Ok {
		t.Fatalf("parent create failed: env=%+v err=%v", env, err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestPreparationPacketCoreTP01SeparateChildProcess$")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		childEnv+"=1",
		"MI_LSP_PREPARATION_TP01_ROOT="+root,
		"MI_LSP_PREPARATION_TP01_PACKET="+packetPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("neutral child verification failed: %v\n%s", err, output)
	}
}
func TestPreparationPacketCoreTP02To14(t *testing.T) {
	tests := []struct {
		name, code string
		mutate     func(*testing.T, *model.PreparationPacket, map[string]any)
	}{
		{"02 explicit workspace identity", "PREPARATION_READY", nil},
		{"03 wrong workspace", "WORKSPACE_MISMATCH", func(_ *testing.T, p *model.PreparationPacket, _ map[string]any) {
			p.Workspace.IdentityDigest = preparationDigest("wrong")
			p.PacketDigest = packetDigest(*p)
		}},
		{"04 task mismatch", "TASK_DIGEST_MISMATCH", func(_ *testing.T, _ *model.PreparationPacket, q map[string]any) { q["task"] = "other" }},
		{"05 expiry injected clock", "PACKET_EXPIRED", func(_ *testing.T, p *model.PreparationPacket, _ map[string]any) {
			p.Lineage.ExpiresAt = time.Now().Add(-time.Hour)
			p.PacketDigest = packetDigest(*p)
		}},
		{"06 governance file drift", "GOVERNANCE_DRIFT", func(t *testing.T, p *model.PreparationPacket, _ map[string]any) {
			writeWorkspaceFile(t, p.Workspace.CanonicalRoot, ".docs/wiki/00_gobierno_documental.md", "drift")
			p.PacketDigest = packetDigest(*p)
		}},
		{"07 index generation drift", "INDEX_DRIFT", func(_ *testing.T, p *model.PreparationPacket, _ map[string]any) {
			p.Semantic.IndexDigest = "sha256:old"
			p.PacketDigest = packetDigest(*p)
		}},
		{"08 plan drift", "PLAN_DRIFT", func(_ *testing.T, p *model.PreparationPacket, q map[string]any) {
			q["plan"] = "changed"
			p.Semantic.PlanDigest = preparationDigest("original")
			p.PacketDigest = packetDigest(*p)
		}},
		{"09 allowlist expansion rejected", "PATH_SCOPE_MISMATCH", func(_ *testing.T, _ *model.PreparationPacket, q map[string]any) {
			q["affected_paths"] = []any{"src/Hello.cs", "secret.txt"}
		}},
		{"10 traversal absolute rejected", "PATH_SCOPE_MISMATCH", func(_ *testing.T, p *model.PreparationPacket, q map[string]any) {
			q["packet_path"] = filepath.Join(filepath.Dir(p.Workspace.CanonicalRoot), "outside.json")
		}},
		{"11 output symlink escape", "PATH_SCOPE_MISMATCH", func(t *testing.T, p *model.PreparationPacket, q map[string]any) {
			outside := t.TempDir()
			link := filepath.Join(p.Evidence.Root, "escape")
			_ = os.Symlink(outside, link)
			q["packet_path"] = link
		}},
		{"12 external evidence root", "PREPARATION_READY", func(t *testing.T, p *model.PreparationPacket, q map[string]any) {
			r := filepath.Join(p.Workspace.CanonicalRoot, "evidence")
			os.MkdirAll(r, 0755)
			p.Evidence.Root = r
			q["evidence_root"] = r
			p.PacketDigest = packetDigest(*p)
		}},
		{"13 tamper rejected", "PACKET_TAMPERED", func(_ *testing.T, p *model.PreparationPacket, _ map[string]any) { p.Task.Intent = "tampered" }},
		{"14 legacy evidence-only fixture rejected by current parser", "PACKET_TAMPERED", func(_ *testing.T, p *model.PreparationPacket, _ map[string]any) {
			p.Schema = "semantic-preparation-evidence/v1"
			p.Compatibility = "legacy"
			p.PacketDigest = packetDigest(*p)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, root, name, p := packetMatrixFixture(t)
			q := map[string]any{"task": "task"}
			if tc.mutate != nil {
				tc.mutate(t, &p, q)
			}
			r := matrixVerify(t, app, root, name, p, q)
			if r.Code != tc.code {
				t.Fatalf("code=%s want %s", r.Code, tc.code)
			}
			if !r.EvidenceOnly || r.Repairability == "" || r.RecommendedAction == "" {
				t.Fatalf("missing evidence-only repair contract: %+v", r)
			}
		})
	}
}
func TestPreparationPacketTransferRequiredAndMissing(t *testing.T) {
	app, root, name, _ := packetMatrixFixture(t)
	for _, tc := range []struct{ hint, code string }{{"hint", "TRANSFER_REQUIRED"}, {"", "PREPARATION_MISSING"}} {
		t.Run(tc.code, func(t *testing.T) {
			r := matrixVerify(t, app, root, name, model.PreparationPacket{}, map[string]any{"parent_transfer_hint": tc.hint, "packet_path": filepath.Join(root, "missing.json")})
			if r.Code != tc.code {
				t.Fatal(r.Code)
			}
		})
	}
}

func TestPreparationPacketTP14RealLegacyEvidenceIsEvidenceOnly(t *testing.T) {
	app, root, name, _ := packetMatrixFixture(t)
	e := model.SemanticPreparationEvidence{Schema: semanticPreparationSchema, WorkspaceRoot: root, TaskDigest: preparationDigest("task"), GovernanceDigest: preparationGovernanceDigest(root), IndexGeneration: preparationIndexGeneration(root), AllowedPaths: []string{"src/Hello.cs"}}
	path := filepath.Join(root, "legacy.json")
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
	env, err := app.verifyPreparation(root, model.CommandRequest{Context: model.QueryOptions{Workspace: name}, Payload: map[string]any{"packet_path": path, "evidence_root": root, "task": "task"}})
	if err != nil {
		t.Fatal(err)
	}
	r := env.Items.([]model.PreparationResult)[0]
	if r.Code != "PREPARATION_READY" || !r.EvidenceOnly || !r.Transferable || r.Packet == nil || r.Packet.Compatibility != "legacy" || !r.Packet.Scope.ReadOnly {
		t.Fatalf("legacy result = %+v", r)
	}
	if r.Packet.Scope.DeniedClasses[0] != "authorization" {
		t.Fatalf("legacy packet elevated: %+v", r.Packet.Scope.DeniedClasses)
	}
}

func TestPreparationPacketTP11OutputThroughDirectoryLinkRejected(t *testing.T) {
	app, root, name, _ := packetMatrixFixture(t)
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "mklink", "/J", link, outside)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Skipf("directory junction unavailable: %v: %s", err, output)
		}
	} else if err := os.Symlink(outside, link); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	out := filepath.Join("linked", "packet.json")
	env, err := app.Execute(t.Context(), model.CommandRequest{Operation: "prepare.create", Context: model.QueryOptions{Workspace: name}, Payload: map[string]any{"task": "task", "output": out, "evidence_root": root}})
	if err != nil {
		t.Fatal(err)
	}
	if got := env.Items.([]model.PreparationResult)[0].Code; got != "PATH_SCOPE_MISMATCH" {
		t.Fatalf("code=%s", got)
	}
	if _, err := os.Stat(filepath.Join(outside, "packet.json")); !os.IsNotExist(err) {
		t.Fatalf("outside output exists: %v", err)
	}
}
