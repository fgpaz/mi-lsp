# Evidencia: release mi-lsp v0.12.11 (E5b)

**Fecha:** 2026-10-14  
**Main:** `e31864ecbc296a373cef64e7c49e56827572caab` (PR #116)  
**Release:** https://github.com/fgpaz/mi-lsp/releases/tag/v0.12.11

## Causa preexistente (v0.12.10 sin release)

El tag `v0.12.10` en `c9c0e33` (merge PR #115, refs Rust) se creó antes de restaurar `scripts/release/metadata`. Goreleaser fallaba en el hook:

```text
stat .../scripts/release/metadata: directory not found
```

**Decisión:** no mover ni borrar `v0.12.10`; publicar **v0.12.11** con metadata y artefactos.

## Reparación (PR #116)

Port mínimo desde `275a33d`:

- `scripts/release/metadata/main.go`
- `integrations/mi-mcp/tool-card.json`
- `internal/model/build_metadata.go` — campo `BuildTime` para ldflags de goreleaser

## Publicación

```bash
scripts/release.sh 0.12.11          # dist/ ok (GOTMPDIR en ~/.cache/gotmp por /tmp lleno en build paralelo)
scripts/release.sh 0.12.11 --upload # 7 assets en GitHub
```

`gh release view v0.12.11` — checksums + 6 archivos por plataforma.

## Verificación instalada (artefacto publicado)

```bash
gh release download v0.12.11 -p '*linux-x64.tar.gz' ...
./mi-lsp version
# version=0.12.11 revision=e31864ecbc29

./mi-lsp nav refs reason_code --backend rust-analyzer \
  --entrypoint rust/mi-rust-molde/Cargo.toml \
  --file rust/mi-rust-molde/src/lib.rs --line 13
# backend: rust-analyzer, origin: semantic
```
