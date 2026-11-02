package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	elasticdaemon "github.com/fgpaz/bots-herramientas/mold/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
)

// healthCache is shared across CLI processes, but never substitutes a status
// request made explicitly by a user. A fresh connection is still required.
func healthProbeClientName(clientName, operation string) string {
	clean := func(raw string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
			if b.Len() >= 32 {
				break
			}
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
				b.WriteRune(r)
			} else if b.Len() == 0 || b.String()[b.Len()-1] != '_' {
				b.WriteByte('_')
			}
		}
		return strings.Trim(b.String(), "_")
	}
	clientName, operation = clean(clientName), clean(operation)
	if clientName == "" && operation == "" {
		return "mi-lsp-daemon-probe"
	}
	if clientName == "" {
		clientName = "internal"
	}
	if operation == "" {
		operation = "health"
	}
	return "mi-lsp-daemon-probe:" + clientName + ":" + operation
}

func ensureElasticHealth(ctx context.Context, probeClient string) (bool, error) {
	node, err := readBabelNode(ctx)
	if err != nil {
		return true, err
	}
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: model.QueryOptions{ClientName: probeClient}}
	var healthy bool
	probe := func() (bool, error) {
		response, used, err := executeNodeScoped(ctx, request, 0)
		healthy = response.Ok
		return used, err
	}
	dir, err := daemonRootDir()
	if err != nil {
		return probe()
	}
	lock, err := os.OpenFile(filepath.Join(dir, "health-elastic-"+node+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return probe()
	}
	defer lock.Close()
	locked, err := tryLockStartGuard(lock)
	if err != nil {
		return probe()
	}
	dial := func() error {
		conn, err := elasticdaemon.DialContext(ctx, "mi-lsp-"+node)
		if err == nil {
			_ = conn.Close()
		}
		return err
	}
	if !locked {
		err = dial()
		if os.IsNotExist(err) {
			return false, nil
		}
		return true, err
	}
	defer unlockStartGuard(lock)
	statePath, err := daemonStatePath()
	if err != nil {
		return probe()
	}
	info, statErr := os.Stat(statePath)
	path := filepath.Join(dir, "health-elastic-"+node+".json")
	var cache healthCache
	body, readErr := os.ReadFile(path)
	if statErr == nil && readErr == nil && json.Unmarshal(body, &cache) == nil && time.Since(cache.Checked) >= 0 && time.Since(cache.Checked) < 70*time.Second && info.ModTime().Equal(cache.StateMod) && info.Size() == cache.StateSize {
		if err := dial(); err == nil {
			return true, nil
		}
	}
	used, err := probe()
	if used && healthy && err == nil {
		if info, statErr = os.Stat(statePath); statErr == nil {
			body, marshalErr := json.Marshal(healthCache{Checked: time.Now(), StateMod: info.ModTime(), StateSize: info.Size()})
			if marshalErr == nil {
				_ = os.WriteFile(path, body, 0600)
			}
		}
	}
	return used, err
}

type healthCache struct {
	Checked   time.Time `json:"checked"`
	StateMod  time.Time `json:"state_mod"`
	StateSize int64     `json:"state_size"`
}

type legacyHealthCache struct {
	healthCache
	StateDigest     string    `json:"state_digest"`
	PID             int       `json:"pid"`
	RunID           int64     `json:"run_id"`
	StartedAt       time.Time `json:"started_at"`
	Endpoint        string    `json:"endpoint"`
	Version         string    `json:"version"`
	ProtocolVersion string    `json:"protocol_version"`
}

// legacyOwnerMatches compares the status observation with a stable on-disk
// owner, including the admin binding without storing the token in the cache.
func legacyOwnerMatches(observed, state model.DaemonState) bool {
	return observed.PID > 0 && observed.RunID > 0 && !observed.StartedAt.IsZero() &&
		observed.Endpoint == defaultEndpoint() && observed.Version != "" && observed.ProtocolVersion != "" &&
		observed.ExecutablePath != "" && observed.ExecutableSHA256 != "" && observed.AdminURL != "" && observed.AdminToken != "" &&
		processExists(observed.PID) && state.PID == observed.PID && state.RunID == observed.RunID &&
		state.StartedAt.Equal(observed.StartedAt) && state.Endpoint == observed.Endpoint && state.Version == observed.Version &&
		state.ProtocolVersion == observed.ProtocolVersion && state.ExecutablePath == observed.ExecutablePath &&
		state.ExecutableSHA256 == observed.ExecutableSHA256 && state.AdminURL == observed.AdminURL && state.AdminToken == observed.AdminToken
}

// writeLegacyHealth must be called under health.lock, and only with a real
// successful status observation. The digest covers the binding but is not a token.
func writeLegacyHealth(dir string, observed model.DaemonState) {
	path, err := daemonStatePath()
	if err != nil {
		return
	}
	state, info, digest, err := readUpdateOwnerState(path)
	if err != nil || !legacyOwnerMatches(observed, state) {
		return
	}
	stateAfter, after, afterDigest, err := readUpdateOwnerState(path)
	if err != nil || !legacyOwnerMatches(observed, stateAfter) || !os.SameFile(info, after) ||
		!info.ModTime().Equal(after.ModTime()) || info.Size() != after.Size() || digest != afterDigest {
		return
	}
	cache := legacyHealthCache{healthCache: healthCache{Checked: time.Now(), StateMod: after.ModTime(), StateSize: after.Size()}, StateDigest: digest,
		PID: state.PID, RunID: state.RunID, StartedAt: state.StartedAt, Endpoint: state.Endpoint, Version: state.Version, ProtocolVersion: state.ProtocolVersion}
	if body, err := json.Marshal(cache); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "health-cache.json"), body, 0600)
	}
}

func rememberLegacyHealthForOwner(observed model.DaemonState) {
	dir, err := daemonRootDir()
	if err != nil {
		return
	}
	lock, err := os.OpenFile(filepath.Join(dir, "health.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return
	}
	defer lock.Close()
	locked, err := tryLockStartGuard(lock)
	if err != nil || !locked {
		return
	}
	defer unlockStartGuard(lock)
	writeLegacyHealth(dir, observed)
}

// On contention do not convert a dial into an authenticated health result.
func probeLegacyOwner(ctx context.Context, probeClient string) (model.DaemonState, error) {
	observed, err := probeDaemonWithTransport(ctx, probeClient, &Client{dial: dialDaemon})
	if err != nil {
		return model.DaemonState{}, err
	}
	path, err := daemonStatePath()
	if err != nil {
		return model.DaemonState{}, err
	}
	state, _, _, err := readUpdateOwnerState(path)
	if err != nil {
		return model.DaemonState{}, err
	}
	if !legacyOwnerMatches(observed, state) {
		return model.DaemonState{}, errors.New("daemon status owner does not match local state")
	}
	return observed, nil
}

func ensureLegacyHealth(ctx context.Context, probeClient string) error {
	dir, err := daemonRootDir()
	if err != nil {
		_, err = probeLegacyOwner(ctx, probeClient)
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "health.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		_, err = probeLegacyOwner(ctx, probeClient)
		return err
	}
	defer lock.Close()
	locked, err := tryLockStartGuard(lock)
	if err != nil {
		_, err = probeLegacyOwner(ctx, probeClient)
		return err
	}
	if !locked {
		_, err := probeLegacyOwner(ctx, probeClient)
		return err
	}
	defer unlockStartGuard(lock)
	statePath, err := daemonStatePath()
	if err != nil {
		_, err = probeLegacyOwner(ctx, probeClient)
		return err
	}
	state, info, digest, stateErr := readUpdateOwnerState(statePath)
	cachePath := filepath.Join(dir, "health-cache.json")
	var cache legacyHealthCache
	body, readErr := os.ReadFile(cachePath)
	if stateErr == nil && readErr == nil && json.Unmarshal(body, &cache) == nil &&
		time.Since(cache.Checked) >= 0 && time.Since(cache.Checked) < 70*time.Second &&
		cache.StateDigest != "" && digest == cache.StateDigest && info.ModTime().Equal(cache.StateMod) && info.Size() == cache.StateSize &&
		cache.PID == state.PID && cache.RunID == state.RunID && cache.StartedAt.Equal(state.StartedAt) &&
		cache.Endpoint == state.Endpoint && cache.Version == state.Version && cache.ProtocolVersion == state.ProtocolVersion && legacyOwnerMatches(state, state) {
		conn, dialErr := dialDaemon(ctx)
		if dialErr == nil {
			_ = conn.Close()
			afterState, after, afterDigest, afterErr := readUpdateOwnerState(statePath)
			if afterErr == nil && os.SameFile(info, after) && info.ModTime().Equal(after.ModTime()) && info.Size() == after.Size() &&
				digest == afterDigest && legacyOwnerMatches(state, afterState) {
				return nil
			}
		}
	}
	observed, err := probeLegacyOwner(ctx, probeClient)
	if err != nil {
		return err
	}
	writeLegacyHealth(dir, observed)
	return nil
}
