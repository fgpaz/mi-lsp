package service

import "testing"

func TestQCursorBindsQueryWorkspaceAndTTL(t *testing.T) {
	token := signQCursor(`sym Run`, `workspace-id`, `generation-1`, 7)
	claims, code := verifyQCursor(token, `sym Run`, `workspace-id`)
	if code != "" || claims.Offset != 7 || claims.Generation != "generation-1" {
		t.Fatalf("cursor claims=%+v code=%s", claims, code)
	}
	if _, code = verifyQCursor(token, `sym Stop`, `workspace-id`); code != "cursor_invalid" {
		t.Fatalf("query mismatch code=%q", code)
	}
	if _, code = verifyQCursor(token, `sym Run`, `other-workspace`); code != "cursor_invalid" {
		t.Fatalf("workspace mismatch code=%q", code)
	}
	if _, code = verifyQCursor("q1.invalid.bad", `sym Run`, `workspace-id`); code != "cursor_invalid" {
		t.Fatalf("bad token code=%q", code)
	}
}
