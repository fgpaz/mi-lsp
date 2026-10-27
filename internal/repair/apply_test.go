package repair

import (
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/repair/actions"
)

func TestApplySuccessPersistsResultAndChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	target := filepath.Join(t.TempDir(), "target")
	old := performAction
	defer func() { performAction = old }()
	performAction = func(kind, target, root, mode string) (actions.Result, error) {
		return actions.Result{Changed: true, Detail: "actualizado"}, nil
	}
	a, result := applyAction(path, "index", target, Options{}, makeAction("index", "planned", "REINDEX_REQUESTED", "", ""))
	if a.Status != "repaired" || !result.Changed {
		t.Fatalf("apply: %+v %+v", a, result)
	}
	s, err := loadState(path)
	if err != nil || len(s.Attempts) != 1 || s.Attempts[0].Result != "success" {
		t.Fatalf("state: %+v %v", s, err)
	}
}
