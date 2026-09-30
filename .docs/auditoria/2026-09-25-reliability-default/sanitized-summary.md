# Cierre fiabilidad harness — resumen derivado

Commit de producto `e583c7dde4d23665c7f5432d268b8f4100da0fbc`. Alineación wiki `f78f1ec34d65e507179e8f00852463a4f5551bd8`.

El CLI humano `manual-cli` sigue con warning si `--workspace` apunta a otro root. Un harness, también si el nombre llega como `root`, `builtin_child`, `mi-lsp-mcp` o un derivado (`pi-chief`, `grok-measure-leaf`), recibe `workspace_cross_workspace_refused` salvo `--allow-cross-workspace`. Un alias desconocido sigue fallando y, si el cwd ya es un workspace, el envelope trae `continuation.next` sin ejecutar esa consulta.

Verificación fresca en `f78f1ec`: `go test ./...` exit 0 y `go build -o bin/mi-lsp.exe ./cmd/mi-lsp` exit 0. Verificador distinto del autor del corte.

El daemon compartido pid 43908 sigue en la imagen `v0.9.0`. No se reinició. El exe nuevo está en `C:\Users\fgpaz\bin\mi-lsp.exe`.
