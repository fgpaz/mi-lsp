package cli

import "testing"

func TestShouldUseDaemonPolicy(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		requested bool
		want      bool
	}{
		{name: "find bypasses daemon", operation: "nav.find", requested: true, want: false},
		{name: "search bypasses daemon", operation: "nav.search", requested: true, want: false},
		{name: "intent bypasses daemon", operation: "nav.intent", requested: true, want: false},
		{name: "symbols bypasses daemon", operation: "nav.symbols", requested: true, want: false},
		{name: "outline bypasses daemon", operation: "nav.outline", requested: true, want: false},
		{name: "overview bypasses daemon", operation: "nav.overview", requested: true, want: false},
		{name: "multi-read bypasses daemon", operation: "nav.multi-read", requested: true, want: false},
		{name: "trace bypasses daemon", operation: "nav.trace", requested: true, want: false},
		{name: "context keeps daemon", operation: "nav.context", requested: true, want: true},
		{name: "refs keeps daemon", operation: "nav.refs", requested: true, want: true},
		{name: "service keeps daemon", operation: "nav.service", requested: true, want: true},
		{name: "workspace-map keeps daemon", operation: "nav.workspace-map", requested: true, want: true},
		{name: "batch keeps daemon", operation: "nav.batch", requested: true, want: true},
		{name: "warm keeps daemon", operation: "workspace.warm", requested: true, want: true},
		{name: "explicit direct stays direct", operation: "nav.context", requested: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUseDaemon(tt.operation, tt.requested); got != tt.want {
				t.Fatalf("shouldUseDaemon(%q, %t) = %t, want %t", tt.operation, tt.requested, got, tt.want)
			}
		})
	}
}

func TestShouldAutoStartDaemonPolicy(t *testing.T) {
	tests := []struct {
		operation string
		want      bool
	}{
		{operation: "nav.find", want: false},
		{operation: "nav.search", want: false},
		{operation: "nav.symbols", want: false},
		{operation: "nav.context", want: true},
		{operation: "nav.refs", want: true},
		{operation: "nav.related", want: true},
		{operation: "nav.deps", want: true},
		{operation: "nav.ask", want: true},
		{operation: "nav.service", want: true},
		{operation: "nav.workspace-map", want: true},
		{operation: "nav.diff-context", want: true},
		{operation: "nav.batch", want: true},
		{operation: "workspace.warm", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			if got := shouldAutoStartDaemon(tt.operation); got != tt.want {
				t.Fatalf("shouldAutoStartDaemon(%q) = %t, want %t", tt.operation, got, tt.want)
			}
		})
	}
}
