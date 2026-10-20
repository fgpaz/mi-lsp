package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// Integration seam test: no root.go registration is needed to exercise the constructor.
func TestFindFilesCommandDashUnicodeAndInheritedBudget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	dir := t.TempDir()
	git := exec.Command("git", "init", "--quiet", dir)
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	for _, name := range []string{"-café file.go", "other.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := &rootState{workspace: dir, format: "json", maxItems: 20}
	root := &cobra.Command{Use: "mi-lsp"}
	root.PersistentFlags().StringVar(&state.format, "format", "json", "")
	root.PersistentFlags().IntVar(&state.maxItems, "max-items", 20, "")
	root.PersistentFlags().IntVar(&state.maxChars, "max-chars", 0, "")
	root.AddCommand(newFindFilesCommand(state))
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"find-files", "--max-items", "1", "--budget-bytes", "1024", "--", "-café file.go"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var result model.FindFilesResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Path != "-café file.go" || result.Budget.Top != 1 || out.Len() > 1024 {
		t.Fatalf("%s", out.Bytes())
	}
}
