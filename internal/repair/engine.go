// Package repair implements the local, opt-in repair plan.
package repair

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/repair/actions"
	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

const Schema = "mi-lsp-repair/v1"
const stateSchema = "mi-lsp-repair-state/v1"
const cooldown = 300 * time.Second

var inspectAction = actions.Inspect
var performAction = actions.Apply

type Action struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	ReasonCode    string     `json:"reasonCode"`
	Detail        string     `json:"detail"`
	Retryable     bool       `json:"retryable"`
	NextAction    string     `json:"nextAction"`
	LastAttemptAt *time.Time `json:"lastAttemptAt"`
	RetryAfter    *time.Time `json:"retryAfter"`
}
type Report struct {
	Schema      string               `json:"schema"`
	OK          bool                 `json:"ok"`
	Mode        string               `json:"mode"`
	Runtime     runtimeinfo.Location `json:"runtime"`
	Actions     []Action             `json:"actions"`
	ReasonCodes []string             `json:"reasonCodes"`
	Changed     bool                 `json:"changed"`
}
type Options struct {
	Workspace, ToolRoot, WorkerMode string
	Apply, Reindex, Worker          bool
}
type attempt struct {
	ID            string    `json:"id"`
	Target        string    `json:"target"`
	LastAttemptAt time.Time `json:"lastAttemptAt"`
	Result        string    `json:"result"`
}
type stateFile struct {
	Schema   string    `json:"schema"`
	Attempts []attempt `json:"attempts"`
}

func statePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mi-lsp", "repair", "state.json"), nil
}
func loadState(path string) (stateFile, error) {
	s := stateFile{Schema: stateSchema, Attempts: []attempt{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&s); err != nil {
		return s, err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return s, errors.New("invalid trailing state data")
	}
	if s.Schema != stateSchema || s.Attempts == nil {
		return s, errors.New("invalid repair state")
	}
	seen := map[string]bool{}
	for _, a := range s.Attempts {
		key := a.ID + "\x00" + a.Target
		if a.ID == "" || !filepath.IsAbs(a.Target) || a.LastAttemptAt.IsZero() || (a.Result != "pending" && a.Result != "failed" && a.Result != "success") || seen[key] {
			return s, errors.New("invalid repair state")
		}
		seen[key] = true
	}
	return s, nil
}
func findAttempt(s stateFile, id, target string) *attempt {
	for i := range s.Attempts {
		if s.Attempts[i].ID == id && s.Attempts[i].Target == target {
			return &s.Attempts[i]
		}
	}
	return nil
}
func targetPath(kind, selector string, loc runtimeinfo.Location) string {
	if kind == "workspace" || kind == "index" {
		if resolution, err := workspace.ResolveWorkspaceSelectionReadOnly(selector, ""); err == nil && resolution.Registration.Root != "" {
			selector = resolution.Registration.Root
		}
		p, err := filepath.Abs(selector)
		if err == nil {
			return filepath.Clean(p)
		}
	}
	if kind == "worker" {
		return filepath.Clean(loc.ManagedRoot)
	}
	if path, err := workspace.RegistryPathReadOnly(); err == nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Dir(loc.ReceiptPath))
}
func makeAction(id, status, code, detail, next string) Action {
	return Action{ID: id, Status: status, ReasonCode: code, Detail: detail, NextAction: next}
}
func (r *Report) add(a Action) {
	r.Actions = append(r.Actions, a)
	if a.Status == "blocked" || a.Status == "failed" {
		r.OK = false
	}
	if a.ReasonCode != "" && a.ReasonCode != "OK" {
		for _, v := range r.ReasonCodes {
			if v == a.ReasonCode {
				return
			}
		}
		r.ReasonCodes = append(r.ReasonCodes, a.ReasonCode)
	}
}

func Run(o Options) Report {
	loc := runtimeinfo.Locate()
	r := Report{Schema: Schema, OK: true, Mode: "plan", Runtime: loc, Actions: []Action{}, ReasonCodes: []string{}}
	if o.Apply {
		r.Mode = "apply"
	}
	if o.WorkerMode == "" {
		o.WorkerMode = "auto"
	}
	path, pathErr := statePath()
	var state stateFile
	var stateErr error
	if pathErr == nil {
		state, stateErr = loadState(path)
	} else {
		stateErr = pathErr
	}
	removed, receiptErr := runtimeinfo.Removed()
	removedRuntime, removedWorker := false, false
	for _, v := range removed {
		if v.Component == "runtime" {
			removedRuntime = true
		}
		if v.Component == "worker" {
			removedWorker = true
		}
	}
	runtimeAction := makeAction("runtime", "pass", "OK", "Ejecutable disponible.", "")
	if loc.ExecutablePath != "" && !strings.EqualFold(filepath.Clean(loc.ExecutablePath), filepath.Clean(loc.ManagedExecutablePath)) {
		runtimeAction.ReasonCode = "RUNTIME_UNMANAGED"
		runtimeAction.Detail = "Ejecutable fuera de la ubicación gestionada."
	}
	if stateErr != nil {
		runtimeAction = makeAction("runtime", "blocked", "REPAIR_STATE_INVALID", "El estado de reparación no es válido o no puede leerse.", "Revisar el estado sin sobrescribirlo.")
	} else if receiptErr != nil {
		runtimeAction = makeAction("runtime", "blocked", "REPAIR_FAILED", "No se pueden leer los recibos de instalación.", "Revisar los recibos antes de continuar.")
	} else if removedRuntime {
		runtimeAction = makeAction("runtime", "blocked", "RUNTIME_REMOVED", "Falta un runtime registrado previamente; no se conoce la causa.", "Revisar la seguridad del endpoint y restaurar mediante el procedimiento autorizado.")
	} else if o.Apply && stateErr == nil && loc.ExecutablePath != "" {
		before, _ := os.ReadFile(loc.ReceiptPath)
		if err := runtimeinfo.Record("runtime", loc.ExecutablePath); err != nil {
			runtimeAction = makeAction("runtime", "failed", "REPAIR_FAILED", "No se pudo registrar el runtime existente.", "Revisar los permisos y los recibos.")
		} else {
			after, _ := os.ReadFile(loc.ReceiptPath)
			r.Changed = !bytes.Equal(before, after)
		}
	}
	r.add(runtimeAction)
	for _, kind := range []string{"config", "workspace", "index", "worker"} {
		if (kind == "workspace" || kind == "index") && strings.TrimSpace(o.Workspace) == "" {
			r.add(makeAction(kind, "skip", "WORKSPACE_REQUIRED", "No se seleccionó workspace.", "Seleccionar un workspace explícito."))
			continue
		}
		if kind == "index" && !o.Reindex {
			r.add(makeAction(kind, "skip", "NOT_REQUESTED", "No se solicitó reindexar.", ""))
			continue
		}
		if kind == "worker" && !o.Worker {
			r.add(makeAction(kind, "skip", "NOT_REQUESTED", "No se solicitó el worker.", ""))
			continue
		}
		if kind == "worker" && (removedWorker || receiptErr != nil) {
			code := "WORKER_REMOVED"
			if receiptErr != nil {
				code = "REPAIR_FAILED"
			}
			r.add(makeAction(kind, "blocked", code, "No se reinstalará el worker sin revisar los recibos.", "Revisar seguridad del endpoint y restauración autorizada."))
			continue
		}
		if stateErr != nil {
			r.add(makeAction(kind, "blocked", "REPAIR_STATE_INVALID", "El estado de reparación no es válido o no puede leerse.", "Revisar el estado sin sobrescribirlo."))
			continue
		}
		result, err := inspectAction(kind, o.Workspace, o.ToolRoot, o.WorkerMode)
		if err != nil {
			code := result.ReasonCode
			if code == "" {
				code = "REPAIR_FAILED"
			}
			r.add(makeAction(kind, "failed", code, result.Detail, result.NextAction))
			continue
		}
		if kind == "config" && result.Needed && result.ReasonCode == "CONFIG_MISSING" && strings.TrimSpace(o.Workspace) != "" {
			r.add(makeAction(kind, "skip", "NOT_REQUESTED", "El registro se creará mediante la acción de workspace cuando sea necesario.", ""))
			continue
		}
		if kind == "index" {
			result.Needed = true
			result.ReasonCode = "REINDEX_REQUESTED"
		}
		if kind == "worker" {
			result.Needed = true
		}
		if !result.Needed {
			r.add(makeAction(kind, "pass", "OK", result.Detail, result.NextAction))
			continue
		}
		a := makeAction(kind, "planned", result.ReasonCode, result.Detail, result.NextAction)
		if a.ReasonCode == "" {
			a.ReasonCode = "REPAIR_FAILED"
		}
		target := targetPath(kind, o.Workspace, loc)
		if previous := findAttempt(state, kind, target); previous != nil {
			t := previous.LastAttemptAt.UTC()
			a.LastAttemptAt = &t
			if previous.Result != "success" {
				retry := t.Add(cooldown)
				if time.Now().Before(retry) {
					a.Status = "blocked"
					a.ReasonCode = "REPAIR_COOLDOWN"
					a.Retryable = true
					a.RetryAfter = &retry
					a.NextAction = "Esperar hasta retryAfter antes de reintentar."
				}
			}
		}
		if o.Apply && a.Status == "planned" {
			a, result = applyAction(path, kind, target, o, a)
			if result.Changed {
				r.Changed = true
			}
		}
		r.add(a)
	}
	return r
}

func applyAction(path, kind, target string, o Options, a Action) (Action, actions.Result) {
	var result actions.Result
	err := runtimeinfo.WithFileLock(path, func() error {
		s, err := loadState(path)
		if err != nil {
			a.Status = "blocked"
			a.ReasonCode = "REPAIR_STATE_INVALID"
			return err
		}
		now := time.Now().UTC()
		old := findAttempt(s, kind, target)
		if old != nil && old.Result != "success" && now.Before(old.LastAttemptAt.Add(cooldown)) {
			retry := old.LastAttemptAt.UTC().Add(cooldown)
			t := old.LastAttemptAt.UTC()
			a.Status = "blocked"
			a.ReasonCode = "REPAIR_COOLDOWN"
			a.Retryable = true
			a.LastAttemptAt = &t
			a.RetryAfter = &retry
			return nil
		}
		entry := attempt{ID: kind, Target: target, LastAttemptAt: now, Result: "pending"}
		if old == nil {
			s.Attempts = append(s.Attempts, entry)
		} else {
			*old = entry
		}
		a.LastAttemptAt = &now
		if err := runtimeinfo.WriteJSONAtomic(path, s); err != nil {
			return err
		}
		result, err = performAction(kind, o.Workspace, o.ToolRoot, o.WorkerMode)
		entry.Result = "success"
		if err != nil {
			entry.Result = "failed"
		}
		*findAttempt(s, kind, target) = entry
		if saveErr := runtimeinfo.WriteJSONAtomic(path, s); saveErr != nil {
			return saveErr
		}
		if err != nil {
			a.Status = "failed"
			a.ReasonCode = result.ReasonCode
			if a.ReasonCode == "" {
				a.ReasonCode = "REPAIR_FAILED"
			}
			a.Detail = result.Detail
			a.NextAction = result.NextAction
			a.Retryable = true
			retry := now.Add(cooldown)
			a.RetryAfter = &retry
			return nil
		}
		a.Status = "repaired"
		a.ReasonCode = "OK"
		a.Detail = result.Detail
		a.NextAction = result.NextAction
		return nil
	})
	if err != nil && a.Status != "blocked" {
		a.Status = "failed"
		a.ReasonCode = "REPAIR_FAILED"
		a.Detail = "No se pudo guardar o bloquear el estado de reparación."
		a.NextAction = "Revisar permisos y estado antes de reintentar."
	}
	return a, result
}
