package service

import (
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

func semanticBackendWarning(backendType string, err error) string {
	if err == nil {
		return fmt.Sprintf("%s unavailable", backendType)
	}
	warning := fmt.Sprintf("%s unavailable: %v", backendType, err)
	if strings.EqualFold(backendType, "roslyn") && shouldSuggestWorkerInstall(err) {
		return warning + ". Run `mi-lsp worker install` to refresh the bundled/global worker."
	}
	return warning
}

func semanticBackendError(backendType string, err error) error {
	if err == nil {
		return fmt.Errorf("%s unavailable", backendType)
	}
	if strings.EqualFold(backendType, "roslyn") && shouldSuggestWorkerInstall(err) {
		return fmt.Errorf("%s unavailable: %v. Run `mi-lsp worker install` to refresh the bundled/global worker.", backendType, err)
	}
	return fmt.Errorf("%s unavailable: %v", backendType, err)
}

func shouldSuggestWorkerInstall(err error) bool {
	if err == nil {
		return false
	}
	return telemetry.IsRoslynWorkerBootstrapText(err.Error())
}
