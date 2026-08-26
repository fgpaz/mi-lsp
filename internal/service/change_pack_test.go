package service

import "testing"

func TestChangePackLiveTargetsPreserveChangedSymbolsAndPaths(t *testing.T) {
	packet := ChangePackPacket{
		ChangedPaths: []string{"src/other.go", "src/service.go", "src/service.go"},
		ChangedSymbols: []DiffSymbol{{File: "src/service.go", Name: "Run"}},
	}
	targets := changePackLiveTargets(packet)
	if len(targets) != 2 || targets[0].path != "src/other.go" || targets[0].symbol != "" || targets[1].path != "src/service.go" || targets[1].symbol != "Run" {
		t.Fatalf("live change targets=%+v", targets)
	}

	unsafe := changePackLiveTargets(ChangePackPacket{ChangedPaths: []string{"../outside.go", "/tmp/outside.go"}})
	if len(unsafe) != 0 {
		t.Fatalf("unsafe changed path entered bridge scope: %+v", unsafe)
	}
}

func TestBuildChangePackContinuationEmitsBatch(t *testing.T) {
	packet := ChangePackPacket{
		ChangedPaths: []string{"internal/service/app.go"},
		ReadFirst: []FlowSliceRead{
			{Path: "internal/service/app.go", Line: 20, Why: "changed"},
		},
	}
	packet.BatchNext = buildChangePackBatchNext(packet)
	cont := buildChangePackContinuation(packet)
	if cont == nil || cont.Next.Op != "nav.batch" {
		t.Fatalf("expected nav.batch continuation, got %#v", cont)
	}
	if len(cont.Next.Batch) == 0 {
		t.Fatal("expected at least one batch op")
	}
}

func TestBuildChangePackReadsPrefersChangedSymbols(t *testing.T) {
	packet := ChangePackPacket{
		ChangedPaths: []string{"internal/service/app.go"},
		ChangedSymbols: []DiffSymbol{
			{File: "internal/service/app.go", Name: "Execute", Line: 51, Kind: "function"},
		},
		Affected: []AffectedItem{
			{Path: "vendor/x.go", Kind: "file", Confidence: 0.9, Reason: "noise"},
			{Path: "internal/service/app.go", Kind: "file", Confidence: 0.6, Reason: "focus"},
		},
	}
	reads := buildChangePackReads(packet, map[string]bool{"vendor/x.go": true}, 5)
	if len(reads) == 0 {
		t.Fatal("expected reads")
	}
	if reads[0].Path != "internal/service/app.go" {
		t.Fatalf("expected focus path first, got %#v", reads)
	}
}
