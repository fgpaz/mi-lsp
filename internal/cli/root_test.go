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
		{name: "pack bypasses daemon", operation: "nav.pack", requested: true, want: false},
		{name: "ask bypasses daemon", operation: "nav.ask", requested: true, want: false},
		{name: "governance bypasses daemon", operation: "nav.governance", requested: true, want: false},
		{name: "context keeps daemon", operation: "nav.context", requested: true, want: true},
		{name: "refs keeps daemon", operation: "nav.refs", requested: true, want: true},
		{name: "service keeps daemon", operation: "nav.service", requested: true, want: true},
		{name: "workspace-map stays direct by default", operation: "nav.workspace-map", requested: true, want: false},
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
		{operation: "nav.pack", want: false},
		{operation: "nav.governance", want: false},
		{operation: "nav.context", want: true},
		{operation: "nav.refs", want: true},
		{operation: "nav.related", want: true},
		{operation: "nav.deps", want: true},
		{operation: "nav.ask", want: false},
		{operation: "nav.service", want: true},
		{operation: "nav.workspace-map", want: false},
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

func TestDaemonOptionsHonorsEnvironmentDefaults(t *testing.T) {
	t.Setenv("MI_LSP_WATCH_MODE", "off")
	t.Setenv("MI_LSP_WATCH_MAX_ROOTS", "3")
	t.Setenv("MI_LSP_DAEMON_MAX_INFLIGHT", "7")

	options, err := daemonOptions("", 0, 0)
	if err != nil {
		t.Fatalf("daemonOptions: %v", err)
	}
	if options.WatchMode != "off" {
		t.Fatalf("WatchMode = %q, want off", options.WatchMode)
	}
	if options.MaxWatchedRoots != 3 {
		t.Fatalf("MaxWatchedRoots = %d, want 3", options.MaxWatchedRoots)
	}
	if options.MaxInflight != 7 {
		t.Fatalf("MaxInflight = %d, want 7", options.MaxInflight)
	}

	options, err = daemonOptions("eager", 5, 9)
	if err != nil {
		t.Fatalf("daemonOptions explicit: %v", err)
	}
	if options.WatchMode != "eager" || options.MaxWatchedRoots != 5 || options.MaxInflight != 9 {
		t.Fatalf("explicit daemonOptions = %+v, want eager/5/9", options)
	}
}

func TestOffsetFromPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    int
		ok      bool
	}{
		{name: "missing", payload: map[string]any{"pattern": "Foo"}, want: 0, ok: false},
		{name: "int", payload: map[string]any{"offset": 3}, want: 3, ok: true},
		{name: "float64", payload: map[string]any{"offset": float64(5)}, want: 5, ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := offsetFromPayload(tt.payload)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("offsetFromPayload(%v) = (%d, %t), want (%d, %t)", tt.payload, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestShouldRecordCLITelemetry(t *testing.T) {
	tests := []struct {
		name  string
		route string
		err   bool
		want  bool
	}{
		{name: "direct records", route: "direct", want: true},
		{name: "direct fallback records", route: "direct_fallback", want: true},
		{name: "daemon success does not double record", route: "daemon", want: false},
		{name: "daemon error still records", route: "daemon", err: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.err {
				err = assertErr{}
			}
			if got := shouldRecordCLITelemetry(tt.route, err); got != tt.want {
				t.Fatalf("shouldRecordCLITelemetry(%q, %v) = %t, want %t", tt.route, err, got, tt.want)
			}
		})
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }
