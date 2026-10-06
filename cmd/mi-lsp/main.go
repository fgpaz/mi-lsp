package main

import (
	"io"
	"os"

	"github.com/fgpaz/mi-lsp/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		if !cli.IsEnvelopePrintedError(err) {
			out := io.Writer(os.Stderr)
			if cli.JSONFormatRequested() {
				out = os.Stdout
			}
			cli.WriteProcessFailure(out, err)
		}
		os.Exit(1)
	}
}
