package service

import "testing"

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
