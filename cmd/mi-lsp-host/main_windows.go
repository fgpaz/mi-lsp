//go:build windows

package main

import (
	"os"

	"github.com/fgpaz/mi-lsp/internal/winhost"
)

func main() { os.Exit(winhost.Main(os.Args[1:])) }
