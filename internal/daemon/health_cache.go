package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	elasticdaemon "github.com/fgpaz/bots-herramientas/mold/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
)

// healthCache is shared across CLI processes, but never substitutes a status
// request made explicitly by a user. A fresh connection is still required.
func ensureElasticHealth(ctx context.Context) (bool, error) {
	node, err := readBabelNode(ctx)
	if err != nil {
		return true, err
	}
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: model.QueryOptions{ClientName: "mi-lsp-daemon-probe"}}
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

// rememberLegacyHealth records the successful startup probe, so the first
// ordinary client does not immediately repeat it. Failure to cache is harmless.
func rememberLegacyHealth() {
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
	statePath, err := daemonStatePath()
	if err != nil {
		return
	}
	info, err := os.Stat(statePath)
	if err != nil {
		return
	}
	body, err := json.Marshal(healthCache{Checked: time.Now(), StateMod: info.ModTime(), StateSize: info.Size()})
	if err == nil {
		_ = os.WriteFile(filepath.Join(dir, "health-cache.json"), body, 0600)
	}
}

func ensureLegacyHealth(ctx context.Context) error {
	dir, err := daemonRootDir()
	if err != nil {
		_, err = probeDaemon(ctx)
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "health.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		_, err = probeDaemon(ctx)
		return err
	}
	defer lock.Close()
	locked, err := tryLockStartGuard(lock)
	if err != nil {
		_, err = probeDaemon(ctx)
		return err
	}
	if !locked {
		// Another process is checking; a live transport suffices for this call.
		conn, err := dialDaemon(ctx)
		if err == nil {
			_ = conn.Close()
		}
		return err
	}
	defer unlockStartGuard(lock)
	statePath, err := daemonStatePath()
	if err != nil {
		_, err = probeDaemon(ctx)
		return err
	}
	info, statErr := os.Stat(statePath)
	cachePath := filepath.Join(dir, "health-cache.json")
	var cache healthCache
	body, readErr := os.ReadFile(cachePath)
	if statErr == nil && readErr == nil && json.Unmarshal(body, &cache) == nil &&
		time.Since(cache.Checked) >= 0 && time.Since(cache.Checked) < 70*time.Second &&
		info.ModTime().Equal(cache.StateMod) && info.Size() == cache.StateSize {
		conn, dialErr := dialDaemon(ctx)
		if dialErr == nil {
			_ = conn.Close()
			return nil
		}
	}
	_, err = probeDaemon(ctx)
	if err != nil {
		return err
	}
	info, err = os.Stat(statePath)
	if err == nil {
		body, marshalErr := json.Marshal(healthCache{Checked: time.Now(), StateMod: info.ModTime(), StateSize: info.Size()})
		if marshalErr == nil {
			_ = os.WriteFile(cachePath, body, 0600)
		}
	}
	return nil
}
