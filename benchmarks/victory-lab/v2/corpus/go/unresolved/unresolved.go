package unresolved

// UsesKnown keeps this package compilable; the missing target is an oracle case.
func UsesKnown(value string) string {
	return value
}
