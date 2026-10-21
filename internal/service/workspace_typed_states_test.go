package service

import "testing"

func TestWorkspaceTypedStatesStayIndependent(t *testing.T) {
	states := workspaceTypedStates(map[string]any{"index_ready": false, "docs_index_ready": true, "docs_ready": false, "governance_blocked": true})
	for _, name := range []string{"runtime", "workspace", "index", "wiki", "policy"} {
		if states[name]["state"] == "" || states[name]["action"] == "" {
			t.Fatalf("missing typed state/action for %s: %+v", name, states)
		}
	}
	if states["index"]["state"] != "index_pending" || states["wiki"]["state"] != "index_pending" || states["policy"]["state"] != "blocked" {
		t.Fatalf("states collapsed or misclassified: %+v", states)
	}
}
