package service

import "strings"

// CanonicalHarnessClient maps the client_name observed on the daemon to a
// harness family. The human default remains manual-cli and is not a harness.
// Telemetry for the mi-pi mismatch (cwd C:\repos\mios\mi-pi, root the Pi clone)
// arrived as root and builtin_child, plus derived names such as pi-root and
// pi-chief. Those normalize here instead of growing an exact-name list.
func CanonicalHarnessClient(clientName string) (family string, ok bool) {
	name := strings.ToLower(strings.TrimSpace(clientName))
	switch name {
	case "", "manual-cli", "cli":
		return "", false
	case "root", "builtin_child":
		return "agent", true
	case "mi-lsp-mcp":
		return "mi-lsp-mcp", true
	}
	for _, family := range []string{
		"claude-code",
		"claude-ai",
		"claude",
		"codex",
		"cursor",
		"grok",
		"opencode",
		"copilot",
		"jetbrains",
		"neovim",
		"emacs",
		"vim",
		"pi",
	} {
		if name == family || strings.HasPrefix(name, family+"-") || strings.HasPrefix(name, family+"_") {
			return family, true
		}
	}
	for _, suffix := range []string{"-leaf", "-chief", "-scout", "-root", "-control-plane"} {
		if strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
			return "agent", true
		}
	}
	return "", false
}

func isHarnessClientName(clientName string) bool {
	_, ok := CanonicalHarnessClient(clientName)
	return ok
}
