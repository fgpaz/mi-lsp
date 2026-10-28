package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const leaseFileLimit = 64 << 10

type LeaseSnapshot struct {
	Schema     string `json:"schema"`
	State      string `json:"state"`
	OwnerPID   int    `json:"owner_pid,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`
	Liveness   string `json:"liveness"`
	Endpoint   string `json:"endpoint,omitempty"`
}

// ReadLeaseSnapshot reads only daemon metadata. It never creates the runtime directory.
func ReadLeaseSnapshot() LeaseSnapshot {
	home, err := os.UserHomeDir()
	if err != nil {
		return leaseIndeterminate()
	}
	return readLeaseSnapshot(filepath.Join(home, ".mi-lsp", "daemon"), probeLeasePID)
}

func leaseIndeterminate() LeaseSnapshot {
	return LeaseSnapshot{Schema: "mi-lsp-daemon-lease/v1", State: "indeterminate", Liveness: "indeterminate"}
}

func readLeaseSnapshot(dir string, probe func(int) string) LeaseSnapshot {
	result := leaseIndeterminate()
	lockPath, statePath := filepath.Join(dir, "instance.lock"), filepath.Join(dir, "state.json")
	firstLock, lockErr := readLeaseFile(lockPath)
	firstState, stateErr := readLeaseFile(statePath)
	if errors.Is(lockErr, os.ErrNotExist) && errors.Is(stateErr, os.ErrNotExist) {
		_, lockErr = readLeaseFile(lockPath)
		_, stateErr = readLeaseFile(statePath)
		if errors.Is(lockErr, os.ErrNotExist) && errors.Is(stateErr, os.ErrNotExist) {
			result.State = "absent"
		} else {
			result.State = "inconsistent"
		}
		return result
	}
	if errors.Is(lockErr, os.ErrNotExist) && stateErr == nil {
		// A state file without the current lease format is an older owner, not proof of absence.
		secondState, secondErr := readLeaseFile(statePath)
		if secondErr != nil || string(firstState) != string(secondState) {
			result.State = "inconsistent"
			return result
		}
		if pid, endpoint, ok := parseLeaseState(firstState); ok {
			result.State, result.OwnerPID, result.Endpoint = "legacy_owner", pid, endpoint
			if probe != nil {
				result.Liveness = probe(pid)
				if result.Liveness != "alive" && result.Liveness != "dead" {
					result.Liveness = "indeterminate"
				}
			}
		} else {
			result.State = "invalid"
		}
		return result
	}
	if lockErr != nil || stateErr != nil {
		if errors.Is(lockErr, os.ErrNotExist) != errors.Is(stateErr, os.ErrNotExist) {
			result.State = "inconsistent"
		}
		return result
	}
	secondLock, lockErr := readLeaseFile(lockPath)
	secondState, stateErr := readLeaseFile(statePath)
	if lockErr != nil || stateErr != nil || string(firstLock) != string(secondLock) || string(firstState) != string(secondState) {
		result.State = "inconsistent"
		return result
	}

	fields, ok := parseLeaseLock(firstLock)
	if !ok {
		result.State = "invalid"
		return result
	}
	pid, pidErr := strconv.Atoi(fields["pid"])
	if pidErr != nil || pid <= 0 || int64(pid) > int64(^uint32(0)>>1) || fields["version"] != "1" || len(fields["nonce"]) != 32 {
		result.State = "invalid"
		return result
	}
	nonce, err := hex.DecodeString(fields["nonce"])
	if err != nil || len(nonce) != 16 {
		result.State = "invalid"
		return result
	}
	statePID, endpoint, validState := parseLeaseState(firstState)
	if !validState {
		result.State = "invalid"
		return result
	}
	result.OwnerPID = pid
	result.Endpoint = endpoint
	if statePID != pid {
		result.State = "inconsistent"
		return result
	}
	sum := sha256.Sum256(nonce)
	result.InstanceID = hex.EncodeToString(sum[:])
	if probe == nil {
		result.State = "indeterminate"
		return result
	}
	result.Liveness = probe(pid)
	switch result.Liveness {
	case "alive":
		result.State = "active"
	case "dead":
		result.State = "stale"
	default:
		result.Liveness = "indeterminate"
		result.State = "indeterminate"
	}
	return result
}

func parseLeaseLock(body []byte) (map[string]string, bool) {
	fields := make(map[string]string, 3)
	for _, line := range strings.Split(string(body), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, false
		}
		if _, exists := fields[parts[0]]; exists {
			return nil, false
		}
		fields[parts[0]] = parts[1]
	}
	if len(fields) != 3 || fields["version"] == "" || fields["pid"] == "" || fields["nonce"] == "" {
		return nil, false
	}
	return fields, true
}

func parseLeaseState(body []byte) (int, string, bool) {
	var state struct {
		PID      int    `json:"pid"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal(body, &state); err != nil || state.PID <= 0 || int64(state.PID) > int64(^uint32(0)>>1) {
		return 0, "", false
	}
	return state.PID, state.Endpoint, true
}

func readLeaseFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("lease metadata is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("lease metadata changed during open")
	}
	body, err := io.ReadAll(io.LimitReader(f, leaseFileLimit+1))
	if err != nil {
		return nil, err
	}
	if len(body) > leaseFileLimit {
		return nil, fmt.Errorf("lease metadata exceeds limit")
	}
	return body, nil
}
