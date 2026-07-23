package callers

import "example.com/mi-lsp/victory-lab-v2/subject"

// Direct creates the shortest inbound call edge to subject.Normalize.
func Direct(value string) string {
	return subject.Normalize(value)
}

// Indirect reaches Normalize through subject.Validate.
func Indirect(value string) string {
	return subject.Validate(value)
}
