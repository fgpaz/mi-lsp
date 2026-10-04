package cli

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestQCommandRegisteredAndArgs(t *testing.T) {
	root := NewRootCommand()
	command, _, err := root.Find([]string{"q"})
	if err != nil {
		t.Fatal(err)
	}
	if command == nil || command.Use != "q <pipeline>" {
		t.Fatalf("q command=%v", command)
	}
}
func TestQEnvelopeContractVersion(t *testing.T) {
	env := model.Envelope{ContractVersion: "q-v1", Operation: "q", Ok: true, Items: []model.QRow{}}
	if env.ContractVersion != "q-v1" {
		t.Fatal("missing contract version")
	}
}
