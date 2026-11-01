// Package actions implementa acciones locales de reparación y su inspección.
package actions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/worker"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type Result struct {
	Needed     bool
	Changed    bool
	ReasonCode string
	Detail     string
	NextAction string
}

func failure(code, detail, next string, err error) (Result, error) {
	if err == nil {
		err = errors.New(detail)
	}
	// Nunca propagar errores con contenido de archivos o salida de procesos.
	return Result{ReasonCode: code, Detail: detail, NextAction: next}, errors.New(detail)
}
func ok(detail string) Result { return Result{ReasonCode: "OK", Detail: detail} }

func inspectRegistry() (model.RegistryFile, bool, error) {
	path, err := workspace.RegistryPathReadOnly()
	if err != nil {
		return model.RegistryFile{}, false, err
	}
	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return model.RegistryFile{Workspaces: map[string]model.WorkspaceRegistration{}}, false, nil
	}
	if err != nil {
		return model.RegistryFile{}, false, err
	}
	reg, err := workspace.LoadRegistryReadOnly()
	return reg, true, err
}

func configResult() (Result, error) {
	_, present, err := inspectRegistry()
	if err != nil {
		return failure("CONFIG_INVALID", "No se pudo leer la configuración del registro.", "Revisar el registro existente sin sobrescribirlo.", err)
	}
	if !present {
		return Result{Needed: true, ReasonCode: "CONFIG_MISSING", Detail: "Falta el registro público.", NextAction: "Crear un registro público vacío o registrar el workspace solicitado."}, nil
	}
	return ok("Registro legible."), nil
}

// resolveSelector sólo consulta el registro y el sistema de archivos; nunca hace GC.
func resolveSelector(target string) (model.WorkspaceRegistration, bool, error) {
	registration, registered, _, err := resolveSelectorWithIdentity(target)
	return registration, registered, err
}

// resolveSelectorWithIdentity conserva el FileInfo del objeto físico seleccionado
// para que Apply pueda compararlo con el descriptor que abre antes de reparar.
func resolveSelectorWithIdentity(target string) (model.WorkspaceRegistration, bool, os.FileInfo, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return model.WorkspaceRegistration{}, false, nil, errors.New("workspace requerido")
	}
	reg, _, err := inspectRegistry()
	if err != nil {
		return model.WorkspaceRegistration{}, false, nil, err
	}
	if item, found := reg.Workspaces[target]; found {
		info, err := os.Stat(item.Root)
		if err != nil || !info.IsDir() {
			return model.WorkspaceRegistration{}, false, nil, errors.New("workspace registrado no disponible")
		}
		item.Name = target
		return item, true, info, nil
	}
	// Un alias desconocido no es una ruta implícita al directorio actual.
	if !filepath.IsAbs(target) && target != "." && target != ".." && !strings.ContainsAny(target, `/\`) {
		return model.WorkspaceRegistration{}, false, nil, errors.New("alias desconocido")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return model.WorkspaceRegistration{}, false, nil, err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return model.WorkspaceRegistration{}, false, nil, errors.New("ruta de workspace no disponible")
	}
	for name, item := range reg.Workspaces {
		other, err := os.Stat(item.Root)
		if err == nil && os.SameFile(info, other) {
			item.Name = name
			return item, true, info, nil
		}
	}
	return model.WorkspaceRegistration{Root: filepath.Clean(abs)}, false, info, nil
}

func Inspect(kind, target, toolRoot, mode string) (Result, error) {
	switch kind {
	case "config":
		return configResult()
	case "workspace", "index":
		if _, _, err := inspectRegistry(); err != nil {
			return failure("CONFIG_INVALID", "No se pudo leer el registro.", "Revisar el registro existente.", err)
		}
		reg, registered, err := resolveSelector(target)
		if err != nil {
			return failure("WORKSPACE_REQUIRED", "Se requiere un workspace o una ruta existente.", "Seleccionar un alias registrado o una ruta de directorio existente.", err)
		}
		if !registered {
			return Result{Needed: true, ReasonCode: "WORKSPACE_UNREGISTERED", Detail: "El workspace no está registrado.", NextAction: "Registrar el workspace seleccionado."}, nil
		}
		if kind == "workspace" {
			plan, err := workspace.PlanWorkspaceMetadataRepair(reg)
			if err != nil {
				return workspaceMetadataFailure(err)
			}
			if !plan.MetadataPresent {
				return ok("Workspace registrado sin project metadata local para reconciliar."), nil
			}
			if !plan.Drift {
				return ok("Metadata de languages coincide con la detección actual."), nil
			}
			return Result{
				Needed:     true,
				ReasonCode: "WORKSPACE_METADATA_DRIFT",
				Detail:     "Los languages detectados difieren de la metadata del workspace registrado.",
				NextAction: "Inspección solo orientativa: --apply vuelve a seleccionar y valida la identidad física; no vincula esta selección. Revisar el delta. --apply es global: persiste repair state y puede registrar el runtime receipt; esta acción solo reconcilia languages. Indexar requiere --reindex explícito.",
			}, nil
		}
		ready, err := catalogReadyReadOnly(reg.Root)
		if err != nil {
			return failure("REPAIR_FAILED", "No se pudo leer el índice existente.", "Revisar el índice sin eliminarlo.", err)
		}
		if !ready {
			return Result{Needed: true, ReasonCode: "INDEX_MISSING", Detail: "No hay un catálogo publicado.", NextAction: "Indexar el workspace seleccionado."}, nil
		}
		return ok("Catálogo publicado disponible."), nil
	case "worker":
		compatible, err := inspectWorker(toolRoot)
		if err != nil {
			return failure("REPAIR_FAILED", "No se pudo inspeccionar el worker.", "Revisar el acceso al worker y su host.", err)
		}
		if !compatible {
			return Result{Needed: true, ReasonCode: "WORKER_MISSING", Detail: "No hay worker compatible disponible.", NextAction: "Instalar un worker compatible."}, nil
		}
		return ok("Worker compatible disponible."), nil
	default:
		return failure("REPAIR_FAILED", "Tipo de acción desconocido.", "Revisar la solicitud.", nil)
	}
}

func Apply(kind, target, toolRoot, mode string) (Result, error) {
	switch kind {
	case "config":
		changed := false
		err := workspace.WithRegistryLock(func() error {
			_, present, err := inspectRegistry()
			if err != nil {
				return err
			}
			if present {
				return nil
			}
			if err := workspace.SaveRegistry(model.RegistryFile{Workspaces: map[string]model.WorkspaceRegistration{}}); err != nil {
				return err
			}
			changed = true
			return nil
		})
		if err != nil {
			return failure("CONFIG_INVALID", "No se pudo crear el registro sin afectar datos existentes.", "Revisar el registro y permisos.", err)
		}
		readback, err := configResult()
		if err != nil || readback.Needed {
			r, e := failure("REPAIR_FAILED", "No se pudo verificar el registro.", "Revisar el registro.", err)
			r.Changed = changed
			return r, e
		}
		readback.Changed = changed
		return readback, nil
	case "workspace":
		return applyWorkspaceMetadata(target)
	case "index":
		return index(target, toolRoot)
	case "worker":
		if mode == "" {
			mode = "auto"
		}
		path, err := worker.InstallWorkerMode(toolRoot, "", mode)
		if err != nil {
			r, e := failure("WORKER_INSTALL_FAILED", "No se pudo instalar o registrar el worker.", "Revisar el host y los permisos antes de reintentar.", err)
			r.Changed = path != ""
			return r, e
		}
		// El propio artefacto instalado debe pasar el handshake, no sólo otro bundle.
		compatible := probeWorker(path)
		if !compatible {
			r, e := failure("WORKER_INSTALL_FAILED", "El worker instalado no superó la verificación.", "Revisar el runtime y el worker instalado.", nil)
			r.Changed = true
			return r, e
		}
		r := ok("Worker instalado y verificado.")
		r.Changed = true
		return r, nil
	default:
		return failure("REPAIR_FAILED", "Tipo de acción desconocido.", "Revisar la solicitud.", nil)
	}
}

func workspaceMetadataFailure(err error) (Result, error) {
	code := "WORKSPACE_METADATA_REPAIR_FAILED"
	detail := "No se pudo verificar o reparar la metadata de languages sin cambiar la identidad del workspace."
	next := "Revisar la metadata y el registro antes de reintentar."
	var partial *workspace.PartialMutationError
	if errors.As(err, &partial) {
		recovery := strings.Join(partial.RecoveryPaths, ", ")
		result, returnedErr := failure(
			"WORKSPACE_METADATA_PARTIAL_MUTATION",
			"La reparación pudo modificar metadata parcialmente; el rollback no quedó verificado.",
			"Inspeccionar y recuperar manualmente: "+recovery+". Verificar identidad física de la raíz antes de escribir.",
			err,
		)
		result.Changed = true
		return result, returnedErr
	}
	switch {
	case errors.Is(err, workspace.ErrWorkspaceMetadataRootChanged):
		code = "WORKSPACE_ROOT_CHANGED"
		detail = "La raíz física seleccionada cambió; no se escribió en el destino nuevo."
		next = "Volver a seleccionar y planificar la raíz verificada antes de reintentar."
	case errors.Is(err, workspace.ErrWorkspaceMetadataUnsafe):
		code = "WORKSPACE_METADATA_UNSAFE"
		detail = "La metadata contiene TOML desconocido o una forma de archivo no segura; no se modificó."
		next = "Resolver explícitamente los campos desconocidos antes de aplicar la reparación."
	case errors.Is(err, workspace.ErrWorkspaceMetadataRegistrationChanged):
		code = "WORKSPACE_REGISTRATION_CHANGED"
		detail = "La identidad registrada del workspace cambió; no se modificó."
		next = "Revisar alias y raíz antes de volver a planificar."
	case errors.Is(err, workspace.ErrWorkspaceMetadataUnavailable):
		code = "WORKSPACE_METADATA_UNAVAILABLE"
		detail = "No se pudo leer o detectar la metadata del workspace; no se modificó."
		next = "Revisar los archivos de metadata y los marcadores visibles del workspace."
	}
	return failure(code, detail, next, err)
}

func applyWorkspaceMetadata(target string) (Result, error) {
	registration, registered, selectedRoot, err := resolveSelectorWithIdentity(target)
	if err != nil {
		return failure("WORKSPACE_REQUIRED", "Se requiere un workspace o una ruta existente.", "Seleccionar un alias registrado o una ruta de directorio existente.", err)
	}
	if !registered {
		return register(target)
	}
	selection := workspace.WorkspaceMetadataSelection{Registration: registration, RootInfo: selectedRoot}
	plan, changed, err := workspace.ApplyWorkspaceMetadataRepairSelected(selection)
	if err != nil {
		result, returnedErr := workspaceMetadataFailure(err)
		result.Changed = result.Changed || changed
		return result, returnedErr
	}
	result := ok("Metadata de languages reconciliada y verificada; no se retargeteó ni indexó el workspace.")
	result.Changed = changed
	result.NextAction = "Este action integra el --apply global, que persiste repair state y puede registrar el runtime receipt; indexación requiere --reindex explícito."
	if !plan.MetadataPresent {
		result.Detail = "Workspace registrado sin project metadata local; no se escribieron archivos del workspace."
	} else if !plan.Drift {
		result.Detail = "Metadata de languages ya coincide; no se escribieron los archivos del workspace."
	}
	return result, nil
}

func register(target string) (Result, error) {
	reg, registered, err := resolveSelector(target)
	if err != nil {
		return failure("WORKSPACE_REQUIRED", "Se requiere una ruta de workspace existente.", "Seleccionar una ruta válida.", err)
	}
	if registered {
		return ok("Workspace ya registrado."), nil
	}
	// Detectar antes del lock; se revalida el registro dentro del lock antes de escribir.
	detected, err := workspace.DetectWorkspace(reg.Root)
	if err != nil {
		return failure("REPAIR_FAILED", "No se pudo detectar el workspace.", "Revisar el workspace seleccionado.", err)
	}
	changed := false
	err = workspace.WithRegistryLock(func() error {
		registry, _, err := inspectRegistry()
		if err != nil {
			return err
		}
		info, err := os.Stat(reg.Root)
		if err != nil {
			return err
		}
		for _, item := range registry.Workspaces {
			other, e := os.Stat(item.Root)
			if e == nil && os.SameFile(info, other) {
				return nil
			}
		}
		// SaveRegistry serializa un modelo tipado; rechazar campos desconocidos
		// evita descartarlos al reescribir una configuración existente.
		path, err := workspace.RegistryPathReadOnly()
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			var check model.RegistryFile
			metadata, err := toml.DecodeFile(path, &check)
			if err != nil || len(metadata.Undecoded()) != 0 {
				return errors.New("registro contiene campos desconocidos")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		name := strings.TrimSpace(detected.Name)
		if name == "" {
			name = filepath.Base(reg.Root)
		}
		if _, exists := registry.Workspaces[name]; exists {
			return errors.New("alias ocupado")
		}
		detected.Name = name
		registry.Workspaces[name] = detected
		registry.Defaults.LastWorkspace = name
		if err := workspace.SaveRegistry(registry); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return failure("REPAIR_FAILED", "No se pudo registrar el workspace sin sobrescribir otro registro.", "Revisar el registro o elegir un alias manualmente.", err)
	}
	_, found, err := resolveSelector(target)
	if err != nil || !found {
		r, e := failure("REPAIR_FAILED", "No se pudo verificar el registro del workspace.", "Revisar el registro.", err)
		r.Changed = changed
		return r, e
	}
	r := ok("Workspace registrado y verificado.")
	r.Changed = changed
	return r, nil
}

func index(target, toolRoot string) (Result, error) {
	reg, registered, err := resolveSelector(target)
	if err != nil || !registered {
		return failure("WORKSPACE_REQUIRED", "El workspace debe estar registrado antes de indexar.", "Registrar el workspace seleccionado.", err)
	}
	// La recuperación automática del servicio puede mover/borrar índices corruptos;
	// no invocarla cuando el índice preexistente no se puede leer.
	dbPath := store.WorkspaceDBPath(reg.Root)
	if _, err := os.Stat(dbPath); err == nil {
		if _, err := store.WorkspaceCatalogReady(context.Background(), reg.Root); err != nil {
			return failure("REPAIR_FAILED", "El índice existente no se puede leer.", "Revisar el índice sin eliminarlo.", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return failure("REPAIR_FAILED", "No se puede inspeccionar el índice.", "Revisar el acceso al índice.", err)
	}
	// Evitar la normalización ordinaria, que puede registrar o hacer GC antes de
	// llegar al resolver por instancia. La selección ya fue validada arriba.
	req := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "index.start", Context: model.QueryOptions{Workspace: reg.Name, NoIndexRefresh: true}, Payload: map[string]any{"path": reg.Name, "mode": "full", "wait": true, "clean": false, "go_offline": true}}
	before, _ := os.Stat(dbPath)
	app := service.New(toolRoot, nil)
	app.ReadOnlyWorkspaceResolution = true
	envelope, err := app.Execute(context.Background(), req)
	if err != nil || !envelope.Ok {
		r, e := failure("REPAIR_FAILED", "No se pudo completar la indexación.", "Revisar el índice y reintentar manualmente.", err)
		after, _ := os.Stat(dbPath)
		r.Changed = after != nil && (before == nil || !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size())
		return r, e
	}
	// Un job existente en curso no es una reindexación completada por esta llamada.
	jobs, okJobs := envelope.Items.([]store.IndexJob)
	if !okJobs || len(jobs) != 1 || jobs[0].Status != store.IndexJobSucceeded {
		return failure("REPAIR_FAILED", "La indexación no confirmó un job completado.", "Esperar al job activo y verificar el catálogo.", nil)
	}
	ready, err := store.WorkspaceCatalogReady(context.Background(), reg.Root)
	if err != nil || !ready {
		r, e := failure("REPAIR_FAILED", "No se pudo verificar el catálogo publicado.", "Revisar el índice.", err)
		r.Changed = true
		return r, e
	}
	return Result{Changed: true, ReasonCode: "OK", Detail: "Índice actualizado y catálogo verificado."}, nil
}
