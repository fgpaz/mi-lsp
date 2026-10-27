package selfupdate

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

type Status struct {
	Schema     string     `json:"schema"`
	Tool       string     `json:"tool"`
	Installed  string     `json:"installed"`
	Latest     string     `json:"latest"`
	LastCheck  *time.Time `json:"last_check"`
	LastResult string     `json:"last_result"`
	Code       *string    `json:"code"`
	Previous   *string    `json:"previous"`
	Channel    string     `json:"channel"`
	Auto       bool       `json:"auto"`
}
type updateCache struct {
	LastCheck time.Time `json:"last_check"`
	Release   Release   `json:"release"`
	Error     string    `json:"error"`
}

func updateDir(home string) string      { return filepath.Join(home, ".mi-lsp", "update") }
func updateLockPath(home string) string { return filepath.Join(updateDir(home), "global") }
func statePath(c Config) (string, error) {
	p, e := filepath.Abs(c.ExecutablePath)
	if e != nil {
		return "", errors.New("invalid executable")
	}
	if resolved, e := filepath.EvalSymlinks(p); e == nil {
		p = resolved
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	h := sha256.Sum256([]byte(p))
	return filepath.Join(updateDir(c.HomeDir), "state", fmt.Sprintf("%x.json", h)), nil
}
func cachePath(c Config) string { return filepath.Join(updateDir(c.HomeDir), "cache.json") }
func readJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 2<<20 {
		return errors.New("state unavailable")
	}
	if json.Unmarshal(b, v) != nil {
		return errors.New("invalid state")
	}
	return nil
}

var allowedCodes = map[string]bool{"UPDATE_NETWORK_UNAVAILABLE": true, "UPDATE_NO_RELEASE": true, "UPDATE_CHECKSUM_MISMATCH": true, "UPDATE_DOWNGRADE_REFUSED": true, "UPDATE_BELOW_FLOOR": true, "UPDATE_LOCKED": true, "UPDATE_IN_USE_DEFERRED": true, "UPDATE_READBACK_FAILED_ROLLED_BACK": true, "UPDATE_QUARANTINED": true, "UPDATE_PERMISSION_DENIED": true}

func validStatus(s Status) bool {
	if s.Schema != "update-status/v1" || s.Tool != "mi-lsp" || s.Installed == "" || (s.Channel != "latest" && s.Channel != "pinned") {
		return false
	}
	switch s.LastResult {
	case "failed", "deferred":
		if s.Code == nil || !allowedCodes[*s.Code] || s.Previous != nil {
			return false
		}
	case "updated":
		if s.Code != nil || s.Previous == nil || *s.Previous == "" || s.Latest == "" || s.Installed != s.Latest {
			return false
		}
	case "up_to_date", "disabled", "pinned":
		if s.Code != nil || s.Previous != nil {
			return false
		}
	default:
		return false
	}
	return true
}
func ReadCached(c Config) (Status, error) {
	p, e := statePath(c)
	if e != nil {
		return Status{}, e
	}
	var raw map[string]json.RawMessage
	if e = readJSON(p, &raw); e != nil {
		return Status{}, e
	}
	for _, k := range []string{"schema", "tool", "installed", "latest", "last_check", "last_result", "code", "previous", "channel", "auto"} {
		if _, ok := raw[k]; !ok {
			return Status{}, errors.New("invalid state")
		}
	}
	b, _ := json.Marshal(raw)
	var s Status
	if len(raw) != 10 || json.Unmarshal(b, &s) != nil || !validStatus(s) {
		return Status{}, errors.New("invalid state")
	}
	return s, nil
}
func saveStatus(c Config, s Status) error {
	if !validStatus(s) {
		return errors.New("invalid status")
	}
	p, e := statePath(c)
	if e != nil {
		return e
	}
	if runtimeinfo.WriteJSONAtomic(p, s) != nil {
		return errors.New("cannot persist status")
	}
	return nil
}
func saveCache(c Config, v updateCache) error {
	if runtimeinfo.WriteJSONAtomic(cachePath(c), v) != nil {
		return errors.New("cannot persist update cache")
	}
	return nil
}
