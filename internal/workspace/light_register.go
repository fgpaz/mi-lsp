package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

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

// LightRegistrationReservation returns an opaque per-generation receipt to the
// starter. Existing/legacy/unknown roots remain busy regardless of age. This is
// a conservative reservation, not proof of a live process or a lifetime lease.
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
	return reserveLightRegistrationAt(context.Background(), reservation)
}

// LightRegistrationFailure consumes a prior error only under the same stable
// guard used for claim/work/finish. A busy root reference has no finish right.
func LightRegistrationFailure(reservation string) string {
	failure, _ := LightRegistrationFailureResult(reservation)
	return failure
}

// LightRegistrationFailureResult distinguishes absent failure from an unknown,
// unreadable or contended receipt. The legacy string wrapper preserves its API.
func LightRegistrationFailureResult(reservation string) (string, error) {
	root, _, valid := lightRegistrationProfileReceipt(reservation)
	if !valid {
		return "", errLightRegistrationOwnerUnknown
	}
	return lightRegistrationFailureResultAt(context.Background(), root)
}

// FinishLightRegistration conditionally finishes only its exact generation;
// legacy root-only references or indeterminate cleanup retain the reservation.
func FinishLightRegistration(reservation string, registrationErr error) error {
	_, nonce, valid := lightRegistrationProfileReceipt(reservation)
	if !valid || nonce == "" {
		return errLightRegistrationOwnerUnknown
	}
	return finishLightRegistrationAt(context.Background(), reservation, registrationErr)
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
