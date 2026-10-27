package cli

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestManagementJSONSurvivesTinyRenderedBudget(t *testing.T) {
	for _, backend := range []string{"version", "doctor"} {
		t.Run(backend, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "stdout-")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			old := os.Stdout
			os.Stdout = f
			defer func() { os.Stdout = old }()
			s := &rootState{}
			err = s.printPreparedEnvelope(model.Envelope{Ok: true, Backend: backend, Items: []any{map[string]any{"runtime": "complete"}}}, model.QueryOptions{Format: "json", MaxChars: 1})
			os.Stdout = old
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(f)
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(data) {
				t.Fatalf("management JSON was cut: %q", data)
			}
		})
	}
}
