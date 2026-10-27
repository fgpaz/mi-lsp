---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-WINDOWS-BUILD
kind: technical
status: active
title: "Construcción Windows y procesos no interactivos"
links:
  code:
    - path: internal/cli/git_frontier.go
    - path: internal/cli/socket.go
    - path: internal/daemon/elastic.go
    - path: internal/daemon/lifecycle_unix.go
    - path: internal/daemon/server_unix.go
    - path: internal/daemon/server_windows.go
    - path: internal/grepx/grepx.go
    - path: internal/indexer/extractor_go.go
    - path: internal/indexer/incremental.go
    - path: internal/milx/isolation_linux.go
    - path: internal/milx/process.go
    - path: internal/milx/process_windows.go
    - path: internal/processutil/command.go
    - path: internal/processutil/command_test.go
    - path: internal/processutil/command_unix_test.go
    - path: internal/processutil/command_windows_test.go
    - path: internal/processutil/exec_guard_test.go
    - path: internal/repair/actions/inspect.go
    - path: internal/rerank/extension.go
    - path: internal/service/diagnostics.go
    - path: internal/service/diff_context.go
    - path: internal/service/edit_plan.go
    - path: internal/service/find_files.go
    - path: internal/service/index_jobs.go
    - path: internal/service/search.go
    - path: internal/userconfig/config.go
    - path: internal/worker/bootstrap.go
    - path: internal/worker/lsp_client.go
    - path: internal/worker/process.go
    - path: internal/worker/tsserver.go
    - path: internal/workspace/autoregister.go
    - path: internal/workspace/registry.go
    - path: internal/workspace/repository_identity.go
---

# Recursos de versión en builds locales de Windows

## Resumen

Los targets `make build-win-x64` y `make build-win-arm64` generan primero recursos PE para ambas arquitecturas mediante `go run github.com/tc-hib/go-winres@v0.3.3`. El generador es una herramienta de build, no una dependencia del runtime; su primera ejecución puede requerir acceso al proxy de módulos Go. Los archivos `cmd/mi-lsp/rsrc_windows_amd64.syso` y `cmd/mi-lsp/rsrc_windows_arm64.syso` son generados e ignorados por Git. Los sufijos limitan su inclusión a la plataforma y arquitectura correspondientes.

Ejemplo: `make build-win-x64 WINDOWS_FILE_VERSION=1.2.3.4` o `make build-win-arm64 WINDOWS_FILE_VERSION=1.2.3.4`. Usar cuatro componentes numéricos compatibles con versiones PE. Si se omite la variable, la versión de desarrollo es `0.0.0.0`. CompanyName es `mi-lsp contributors` y ProductName es `mi-lsp`. `make windows-resources WINDOWS_FILE_VERSION=1.2.3.4` genera los recursos sin construir el ejecutable.

Un `go build` directo no genera recursos: solo los incluye si ya existen en el directorio del paquete, por lo que puede incluir una versión previamente generada y obsoleta. Usar los targets anteriores para regenerarlos. Esta integración no cambia la publicación ni los scripts de distribución. Los metadatos no son firma de código y no garantizan que Defender deje de marcar el ejecutable.

## Mapa de componentes

### Procesos no interactivos

Los procesos de producción se crean con `processutil.Command` o `processutil.CommandContext`, con la misma firma y cancelación que `os/exec`. En Windows configuran `HideWindow` y `CREATE_NO_WINDOW`; en Unix no agregan atributos. `ConfigureDetachedCommand` conserva atributos existentes y agrega la configuración de proceso separado correspondiente a la plataforma. La apertura de URL del administrador es la única excepción de interfaz, limitada a `openURL`.

El guard AST de `internal/processutil` controla las referencias a los constructores de `os/exec`, incluso alias y valores de función. El probe Windows ejecuta un padre detached propio, comprueba `GetConsoleWindow` del padre y de un hijo consola y ejecuta git configurado. No mide directamente la ventana de git ni demuestra por sí solo ausencia de flashes en todos los procesos.
