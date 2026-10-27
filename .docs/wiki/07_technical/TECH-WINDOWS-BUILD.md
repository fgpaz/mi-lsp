# Recursos de versión en builds locales de Windows

Los targets `make build-win-x64` y `make build-win-arm64` generan primero recursos PE para ambas arquitecturas mediante `go run github.com/tc-hib/go-winres@v0.3.3`. El generador es una herramienta de build, no una dependencia del runtime; su primera ejecución puede requerir acceso al proxy de módulos Go. Los archivos `cmd/mi-lsp/rsrc_windows_amd64.syso` y `cmd/mi-lsp/rsrc_windows_arm64.syso` son generados e ignorados por Git. Los sufijos limitan su inclusión a la plataforma y arquitectura correspondientes.

Ejemplo: `make build-win-x64 WINDOWS_FILE_VERSION=1.2.3.4` o `make build-win-arm64 WINDOWS_FILE_VERSION=1.2.3.4`. Usar cuatro componentes numéricos compatibles con versiones PE. Si se omite la variable, la versión de desarrollo es `0.0.0.0`. CompanyName es `mi-lsp contributors` y ProductName es `mi-lsp`. `make windows-resources WINDOWS_FILE_VERSION=1.2.3.4` genera los recursos sin construir el ejecutable.

Un `go build` directo no genera recursos: solo los incluye si ya existen en el directorio del paquete, por lo que puede incluir una versión previamente generada y obsoleta. Usar los targets anteriores para regenerarlos. Esta integración no cambia la publicación ni los scripts de distribución. Los metadatos no son firma de código y no garantizan que Defender deje de marcar el ejecutable.
