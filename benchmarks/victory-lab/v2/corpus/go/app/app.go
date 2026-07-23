package app

import "example.com/mi-lsp/victory-lab-v2/callers"

// Run supplies a transitive caller and a deterministic shortest path.
func Run(value string) string {
	return callers.Direct(value) + callers.Indirect(value)
}
