package main

import (
	"os"

	"github.com/fgpaz/mi-lsp/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		if !cli.IsEnvelopePrintedError(err) {
			cli.WriteProcessFailure(os.Stderr, err)
		}
		os.Exit(1)
	}
}
