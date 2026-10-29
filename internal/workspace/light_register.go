package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// NeedsLightRegistration performs a read-only precheck; the writer rechecks under lock.
func NeedsLightRegistration(selector, cwd string) (bool, error) {
	if AutoRegisterDisabledByEnv() {
		return false, nil
	}
	root, ok := autoRegisterCandidateRoot(selector, cwd)
	if !ok {
		return false, nil
	}
	if linked, _, valid := linkedWorktree(root); valid && linked {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(AutoRegisterModeEnvVar)), "force") && !autoRegisterDefaultRootEligible(root) {
		return false, nil
	}
	registry, err := LoadRegistryReadOnly()
	if err != nil {
		return false, err
	}
	_, _, found := registeredWorkspaceForRoot(root, registry)
	return !found, nil
}

// LightRegistrationReservation coordinates async registration workers across processes.
// The reservation is a mkdir-based lease keyed by canonical git root.
func LightRegistrationReservation(selector, cwd string) (string, bool, error) {
	root, ok := autoRegisterCandidateRoot(selector, cwd)
	if !ok {
		return "", false, nil
	}
	registryPath, err := RegistryPath()
	if err != nil {
		return "", false, err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	reservation := filepath.Join(filepath.Dir(registryPath), ".light-register-"+hex.EncodeToString(sum[:12]))
	if err := os.Mkdir(reservation, 0o700); err == nil {
		return reservation, true, nil
	} else if !os.IsExist(err) {
		return "", false, err
	}
	// Recover a reservation left behind by a crashed worker; never steal a live one.
	info, err := os.Stat(reservation)
	if err != nil {
		return "", false, err
	}
	if time.Since(info.ModTime()) > 2*time.Minute {
		_ = os.Remove(reservation + ".failure")
		if err := os.Remove(reservation); err == nil {
			if err := os.Mkdir(reservation, 0o700); err == nil {
				return reservation, true, nil
			}
		}
	}
	return reservation, false, nil
}

// LightRegistrationFailure returns and clears a prior asynchronous worker failure.
func LightRegistrationFailure(reservation string) string {
	data, err := os.ReadFile(reservation + ".failure")
	if err != nil {
		return ""
	}
	_ = os.Remove(reservation + ".failure")
	return strings.TrimSpace(string(data))
}

// FinishLightRegistration releases the cross-process reservation and preserves failures for the next query to report.
func FinishLightRegistration(reservation string, registrationErr error) {
	if reservation == "" {
		return
	}
	if registrationErr != nil {
		_ = os.WriteFile(reservation+".failure", []byte(registrationErr.Error()), 0o600)
	}
	_ = os.Remove(reservation)
}

// RegisterGitRootLight only updates the registry. It never creates project files or indexes.
func RegisterGitRootLight(selector, cwd string) (bool, error) {
	if AutoRegisterDisabledByEnv() {
		return false, nil
	}
	root, ok := autoRegisterCandidateRoot(selector, cwd)
	if !ok {
		return false, nil
	}
	if linked, _, valid := linkedWorktree(root); valid && linked {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(AutoRegisterModeEnvVar)), "force") && !autoRegisterDefaultRootEligible(root) {
		return false, nil
	}
	registered := false
	err := WithRegistryLock(func() error {
		registry, err := LoadRegistry()
		if err != nil {
			return err
		}
		if _, _, found := registeredWorkspaceForRoot(root, registry); found {
			return nil
		}
		if registry.Workspaces == nil {
			registry.Workspaces = map[string]model.WorkspaceRegistration{}
		}
		alias := autoRegisterAlias(root, registry)
		registry.Workspaces[alias] = model.WorkspaceRegistration{Name: alias, Root: filepath.Clean(root), Kind: model.WorkspaceKindSingle}
		if err := SaveRegistry(registry); err != nil {
			return err
		}
		registered = true
		return nil
	})
	return registered, err
}
