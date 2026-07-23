package milx

// newIsolationGuard is package-only so tests can exercise Host without being
// able to forge the production proof from another package.
func newIsolationGuardForTest() *IsolationGuard {
	return &IsolationGuard{networkDenied: true, processTreeContained: true}
}
