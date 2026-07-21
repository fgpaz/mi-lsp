package subject

// Normalize is the stable graph subject used by callers and path queries.
func Normalize(value string) string {
	return value
}

// Validate adds a second call edge to Normalize for transitive queries.
func Validate(value string) string {
	return Normalize(value)
}

// Uncalled is a negative control with no callers in the fixture.
func Uncalled(value string) string {
	return value
}
