package selfupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Only an explicit environment pin or a valid pin marker opts out; installation paths are not policy.
func updatePolicy(c Config) (channel string, auto bool, reason string, err error) {
	channel = "latest"
	auto = os.Getenv("MI_LSP_AUTO_UPDATE") != "0"
	if os.Getenv("MI_LSP_CHANNEL") == "pinned" {
		return "pinned", auto, "pinned", nil
	}
	p := filepath.Join(updateDir(c.HomeDir), "pin.json")
	b, e := os.ReadFile(p)
	if e == nil {
		var v struct {
			Schema string `json:"schema"`
			Pinned bool   `json:"pinned"`
		}
		if len(b) > 4096 || json.Unmarshal(b, &v) != nil || v.Schema != "mi-lsp-update-pin/v1" {
			return "", false, "", errors.New("invalid pin marker")
		}
		if v.Pinned {
			return "pinned", auto, "pinned", nil
		}
	} else if !os.IsNotExist(e) {
		return "", false, "", errors.New("cannot read policy")
	}
	if !auto {
		return channel, auto, "disabled", nil
	}
	return channel, auto, "", nil
}
