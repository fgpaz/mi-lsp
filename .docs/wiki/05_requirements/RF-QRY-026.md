---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-026
kind: requirement
status: active
title: Búsqueda y edición semántica nativa de símbolos
links:
  implements:
  - FL-QRY-01
  code:
  - path: integrations/mi-mcp/provider-manifest.json
    role: contract
  - path: internal/cli/nav.go
    role: behavior
  - path: internal/cli/provider_manifest.go
    role: contract
  - path: internal/cli/provider_manifest_test.go
    role: test
  - path: internal/cli/root.go
    role: behavior
  - path: internal/daemon/file_watcher.go
    role: behavior
  - path: internal/daemon/lifecycle.go
    role: behavior
  - path: internal/daemon/server.go
    role: behavior
  - path: internal/model/types.go
    role: contract
  - path: internal/service/app.go
    role: behavior
  - path: internal/service/diagnostics.go
    role: behavior
  - path: internal/service/semantic_refs.go
    role: behavior
  - path: internal/worker/lsp_client.go
    role: behavior
  - path: internal/cli/native_verbs.go
    role: behavior
  - path: internal/model/native_symbol_selector.go
    role: behavior
  - path: internal/service/native_verbs.go
    role: behavior
  - path: internal/worker/lsp_rename.go
    role: behavior
---

## Propósito

Exponer operaciones nativas de búsqueda de símbolos, diagnósticos semánticos y rename explícito de símbolos Go, conservando selección verificable, aislamiento del workspace y estados incompletos visibles. El rename requiere una invocación explícita del consumidor; no habilita edición autónoma.

## Comportamiento ordenado

1. La CLI expone `find_symbol`, `rename_symbol` y `get_errors` en el root y bajo `nav`; el proveedor local publica sus aliases y schemas sobre esos mismos comandos.
2. `find_symbol` combina candidatos acotados del catálogo con definición, referencias semánticas y rangos de fuente cuando el backend elegido está disponible. Un resultado de catálogo, frío, ambiguo, truncado o degradado se marca como aproximado/parcial; no se presenta como una resolución semántica exacta ni selecciona un símbolo arbitrariamente.
3. `rename_symbol` acepta identificadores Go distintos y requiere `--new-name`. Solo usa `gopls`. Un nombre sin posición solicita candidatos en vivo; ninguno se renombra hasta que el consumidor reenvía una selección validada. `candidate.symbol` es un selector opaco, versionado y no confiable: antes de editar se revalidan workspace y repo físicos, archivo/ruta, snapshot fuente y `prepareRename` en vivo. La selección explícita alternativa exige `--file`, `--line` y `--column` juntos; línea y columna son 1-based y columna usa UTF-16. En workspaces con varios repos el scope semántico debe quedar resuelto explícitamente.
4. El core aplica únicamente un `WorkspaceEdit` completo con `documentChanges` versionados. Verifica que la edición coincide con el símbolo y el conjunto completo de referencias, que los snapshots no cambiaron y que cada URI es local. Rechaza authorities de URI remotas, escapes del workspace/repo, symlinks y `.git`; una URI no local nunca se reinterpreta como path local. La aplicación es atómica por archivo y revierte archivos ya aplicados ante fallas detectadas; si la recuperación no es completa informa mutación parcial y artefactos de recuperación. El límite es 32 archivos y 5000 edits.
5. `get_errors` devuelve diagnósticos semánticos para paths explícitos o archivos fuente cambiados. `checked`, `partial` y `not_checked` describen cobertura; después de rename también se devuelven `files_touched` y `diagnostics_status`. En todos los casos `compilation_state` permanece `not_checked`: los diagnósticos no son una compilación del proyecto.

## Superficies

- CLI root: `mi-lsp find_symbol`, `mi-lsp rename_symbol`, `mi-lsp get_errors`.
- CLI nav: `mi-lsp nav find_symbol`, `mi-lsp nav rename_symbol`, `mi-lsp nav get_errors`.
- Proveedor local: aliases `find_symbol`, `rename_symbol` y `get_errors`, ejecutados por la CLI.
- RPC local CLI-daemon/core y worker LSP `gopls` para rename Go; el catálogo mantiene resultados aproximados cuando no hay semántica completa.

## Entradas/salidas tipadas

- `find_symbol`: `query` requerido y `repo` opcional; contrato `find_symbol/v0`. La respuesta separa `definitions`, `usages`, `ranges`, `source`, `semantic_status`, `approximate` y `ambiguity`.
- `rename_symbol`: `symbol` y `new_name` requeridos; scope opcional (`repo`, `entrypoint`, `solution`, `project_path`). La selección por posición requiere el conjunto completo `file`, `line`, `column`; el símbolo opaco devuelto por una respuesta de candidatos se vuelve a enviar en `symbol`. Contrato `rename_symbol/v0`; expone `files_touched`, `application`, `diagnostics_status`, `compilation_state`, y, si corresponde, `mutation_state`, `partial_mutation` y `recovery_artifacts`.
- `get_errors`: paths posicionales o `--file` repetible; sin paths explícitos consulta archivos fuente cambiados. `--base` solo aplica al default de cambios; `--max-files` está acotado a 1–64. Contrato `get_errors/v0`; informa `status`, cobertura y omisiones.
- Los requests daemon usan `protocol_version=mi-lsp-v1.1`, `operation`, `context` y `payload`; los nombres de operación son idénticos a los verbos.

## Errores

### E1 · invalid_input
«rename requires distinct Go identifiers and --new-name; explicit selection requires file, 1-based line, and UTF-16 column together»

### E2 · selection_required
«workspace/symbol completeness is unproven; select one prepareRename-validated candidate.symbol and retry; no files changed»

### E3 · invalid_path
«native rename rejects non-local file URI authorities and userinfo»

### E4 · explicit_incomplete
«gopls worker is cold; rename was not attempted and no files changed»

### E5 · unsupported_operation
«gopls did not provide a complete versioned semantic rename plan; no files changed»

### E6 · unavailable
«semantic backend is cold; diagnostics were not started and no clean result is inferred»

### E7 · mutation_recovery_failed
«rename artifacts could not be cleaned; inspect recovery paths»

## Criterios de aceptación

1. Los verbos root y sus aliases `nav` comparten los nombres RPC y los contratos publicados; los argumentos mínimos son `find_symbol <query>`, `rename_symbol <symbol> --new-name <identifier>` y `get_errors [file ...]`. El provider-manifest describe las mismas entradas/salidas, sin añadir una puerta MCP residente.
2. `find_symbol` solo afirma definición/referencias exactas cuando su evidencia semántica es completa y no degradada; catálogo, worker frío, ambigüedad, truncación o backend no compatible quedan marcados como aproximados/parciales y nunca eligen un objetivo arbitrario.
3. `rename_symbol` rechaza nombres inválidos/iguales, exige `gopls`, y ante selección ausente, ambigua, incompleta, fría o no verificable devuelve estado tipado con `files_touched=[]`; un selector opaco solo se acepta después de revalidar workspace/repo físicos, snapshot, ruta y `prepareRename` en vivo. Coordenadas explícitas son un selector todo-o-nada con columna UTF-16 1-based.
4. Solo se aplica el plan versionado completo cuya lista de archivos coincide exactamente con las referencias verificadas. Se rechazan URI con authority no local, paths fuera del workspace/repo, symlinks, `.git`, snapshots/versiones stale, ediciones que no coinciden con el símbolo o planes incompletos; no se modifica ningún archivo en esos rechazos. Ante falla de aplicación, el resultado declara honestamente rollback, mutación parcial y artefactos de recuperación.
5. Un rename exitoso puede cambiar varios archivos del scope validado y devolver readback/diagnósticos parciales; `get_errors` informa cobertura `checked|partial|not_checked`. Ninguna respuesta declara que el proyecto compiló: `compilation_state=not_checked` se conserva incluso si un fixture aislado compiló.
