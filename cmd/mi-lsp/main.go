package main

import (
	"fmt"
	"os"

	"github.com/fgpaz/mi-lsp/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		if !cli.IsEnvelopePrintedError(err) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
