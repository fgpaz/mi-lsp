package runtimeinfo

import (
	"os"
	"path/filepath"
	"runtime"
)

// Location describes paths without asserting that an installation exists.
type Location struct {
	ExecutablePath        string `json:"executablePath"`
	ManagedRoot           string `json:"managedRoot"`
	ManagedExecutablePath string `json:"managedExecutablePath"`
	ReceiptPath           string `json:"receiptPath"`
}

func Locate() Location {
	home, _ := os.UserHomeDir()
	executable, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	if executable != "" {
		executable = filepath.Clean(executable)
	}
	base := ""
	name := "mi-lsp"
	if runtime.GOOS == "windows" {
		base = os.Getenv("LOCALAPPDATA")
		if !filepath.IsAbs(base) && filepath.IsAbs(home) {
			base = filepath.Join(home, "AppData", "Local")
		}
		name += ".exe"
	} else {
		base = os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(base) && filepath.IsAbs(home) {
			base = filepath.Join(home, ".local", "share")
		}
	}
	result := Location{ExecutablePath: executable}
	if filepath.IsAbs(base) {
		result.ManagedRoot = filepath.Join(base, "mi-lsp-runtime")
		result.ManagedExecutablePath = filepath.Join(result.ManagedRoot, name)
	}
	if filepath.IsAbs(home) {
		result.ReceiptPath = filepath.Join(home, ".mi-lsp", "repair", "installations.json")
	}
	return result
}
