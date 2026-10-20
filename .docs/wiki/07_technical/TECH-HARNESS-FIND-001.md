---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-HARNESS-FIND-001
kind: technical
status: draft
title: "Listado nativo cacheable y ranking acotado de nombres"
links:
  supports: [RF-HARNESS-FIND-001]
  code:
    - path: internal/cli/find_files.go
      role: behavior
    - path: internal/service/find_files.go
      role: behavior
    - path: internal/model/find_files.go
      role: behavior
---

## Resumen
`newFindFilesCommand(state *rootState) *cobra.Command` es el constructor de la lane. La superficie raíz registra ese comando y publica su manifiesto; no se cambia `root.go`, `nav.go` ni `provider_manifest.go` desde esta implementación.

Servicio: `service.NewFileFinder() *FileFinder`; método `(*FileFinder).FindFiles(context.Context, root, query string, FindFilesOptions) (model.FindFilesResult, error)`. Opciones: `Top int`, `MaxBytes int`, `Literal bool`, `Refresh bool`. `FileFinder.CacheDir` permite desactivar la caché con cadena vacía o aislarla en fixtures. El constructor CLI no pasa por `App.Execute`, daemon ni refresh semántico. La configuración de usuario se conserva fuera del repo.

## Enumeración sin shell
La consulta se procesa exclusivamente en Go. Los argv nativos son constantes:

```text
git rev-parse --is-inside-work-tree
git ls-files -z --cached --others --exclude-standard -- .
rg --files --null --hidden --no-require-git --glob !.git -- .
fd --type f --hidden --no-require-git --print0 --exclude .git -- . .
```

Git tiene prioridad dentro de un worktree; si no aplica se usa rg, luego fd/fdfind. No se requiere fd. Las listas NUL soportan espacios y saltos de línea sin separar nombres incorrectamente. Se deduplican rutas y se descartan nombres inválidos. Git conserva versionados ignorados; rg/fd también pueden aplicar sus archivos `.ignore`/`.rgignore`/`.fdignore`. No se promete una identidad de inventario entre motores. Ante un error del enumerador seleccionado no se oculta el fallo recorriendo todo con otro motor.

No se reutiliza el matcher de indexación de `workspace/gitignore.go`: añade ignores propios del indexador y no equivale a Git anidado. Sin enumerador nativo disponible, el error tipado evita una promesa falsa de equivalencia. Todas las plataformas usan `exec.CommandContext` con argv, no `sh`, PowerShell ni cmd.exe.

## Caché y presupuesto
Snapshot JSON v1 bajo `os.UserCacheDir()/mi-lsp/find-files-v1/<sha256(root físico)>.json`, creado mediante temporal y rename best-effort. TTL fijo de cinco segundos, sin watcher ni servicio. La caché contiene solo rutas/raíz del workspace del usuario; no contenido. Un fallo de escritura no hace fallar la búsqueda. Windows puede rechazar reemplazar un destino existente: la consulta sigue correcta y eventualmente reenumera, sin afirmar un cache hit.

La edad se calcula desde el final de la enumeración, que no es un snapshot transaccional del sistema de archivos. Los cambios de ignores/altas dentro del TTL requieren `--refresh`. Un snapshot vencido, malformado o de otra raíz no se usa. La lista nativa se limita a 32 MiB; la operación completa vence a los diez segundos. No hay total global costoso: `matched` cuenta solo coincidencias del snapshot que siguen siendo archivos regulares accesibles. Los symlinks finales y ancestros se descartan; comprobaciones de directorios se memoizan dentro de la consulta. Como cualquier enumeración, no garantiza atomicidad frente a renames concurrentes.

Un max-heap conserva como máximo `top` coincidencias, con orden determinista. El glob usa `path.Match`; `**` entre segmentos se resuelve con memoización, sin regex de usuario. Tras ordenar, se serializa el envelope completo y se recortan ítems hasta respetar `MaxBytes`, incluida nueva línea. Nunca se corta JSON por mitad.

## JSON público y errores
Respuesta `find-files/v1`: `schema`, `items:[{path,rank,reason}]`, `matched`, `truncated`, `truncation_reason?`, `next_hint?`, `budget:{top,max_bytes}`, `listing:{backend,cached,age_ms,max_age_ms}`. Los motivos de truncamiento son `top` y `budget_bytes`; si ambos limitan, prima `budget_bytes`. Los motivos de ranking son `basename_exact`, `basename_prefix`, `segment`, `substring` y `glob`.

Error CLI: `{error:{code,message}}` y estado no cero. Códigos: `invalid_budget`, `invalid_pattern`, `invalid_workspace`, `cancelled`, `listing_timeout`, `file_listing_unavailable`, `file_listing_failed`, `unsupported_format`, `find_files_failed`. No se devuelve stderr ni argv privado. El único formato de esta superficie es JSON, aun cuando el formato global implícito sea compact/agent. Un formato no JSON explícito se rechaza.

## Mapa de componentes
- `newFindFilesCommand(state *rootState)` registra la superficie; `service.NewFileFinder()` y `(*FileFinder).FindFiles(...)` ejecutan la búsqueda.
- La enumeración prioriza Git y conserva las alternativas rg/fd; la caché JSON vive fuera del repositorio.
- `internal/cli/find_files.go`, `internal/service/find_files.go` e `internal/model/find_files.go` son los componentes declarados en `links.code`.

## Enlaces de integración
RF fuente: [[RF-HARNESS-FIND-001]]. El FL común debe declarar `fulfills` hacia el RS común y el RF declarar `implements` hacia ese FL, una vez conocidos los IDs por el dueño de surface. [[TP-HARNESS-FIND-001]] contiene la receta de la FINAL_VERIFY del padre; no se ejecuta por lane.
