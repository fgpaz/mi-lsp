package cli

import (
	"runtime"
	"strings"
)

// Keep the actual argv: rebuilding it from a symbol loses flags such as
// --callers, explicit file sets, source ranges and workspace selection.
func expandedCommand(args []string) string {
	parts := []string{"mi-lsp"}
	for _, arg := range args {
		if runtime.GOOS == "windows" {
			parts = append(parts, "'"+strings.ReplaceAll(arg, "'", "''")+"'")
		} else {
			parts = append(parts, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
		}
	}
	return strings.Join(parts, " ") + " --profile human --full --format json --max-items 200 --max-chars 65536 --token-budget 16000"
}
