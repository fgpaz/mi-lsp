package service

import "testing"

func TestHarnessUsefulnessScoreDemotesNoiseAndHubs(t *testing.T) {
	focus := []string{"internal/service/app.go"}
	symbols := []string{"Execute"}

	impl := harnessUsefulnessScore("internal/service/app.go", "function", "Execute", 0.8, focus, symbols, false)
	test := harnessUsefulnessScore("internal/service/app_test.go", "function", "TestExecute", 0.8, focus, symbols, false)
	vendor := harnessUsefulnessScore("vendor/github.com/x/y.go", "function", "Helper", 0.9, focus, symbols, false)
	hub := harnessUsefulnessScore("internal/model/types.go", "type", "Envelope", 0.9, focus, symbols, true)

	if !(impl > test && impl > vendor && impl > hub) {
		t.Fatalf("expected impl highest, got impl=%.2f test=%.2f vendor=%.2f hub=%.2f", impl, test, vendor, hub)
	}
}

func TestRankAffectedForHarnessOrdersByUsefulness(t *testing.T) {
	items := []AffectedItem{
		{Kind: "file", Path: "vendor/lib/x.go", Confidence: 0.9, Reason: "noise"},
		{Kind: "file", Path: "internal/service/app.go", Confidence: 0.7, Reason: "focus"},
		{Kind: "test", Path: "internal/service/app_test.go", Confidence: 0.8, Reason: "test"},
	}
	ranked := rankAffectedForHarness(items, []string{"internal/service/app.go"}, map[string]bool{})
	if ranked[0].Path != "internal/service/app.go" {
		t.Fatalf("expected focus path first, got %#v", ranked)
	}
}

func TestIsNoisePath(t *testing.T) {
	if !isNoisePath("vendor/foo/bar.go") {
		t.Fatal("vendor should be noise")
	}
	if isNoisePath("internal/service/app.go") {
		t.Fatal("internal should not be noise")
	}
}
