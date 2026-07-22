---
linear_parent: not_applicable_user_authorized
linear_child: not_applicable_user_authorized
anchors: [FL-WIKI-01, RF-QRY-016, RF-WIKI-005, CT-NAV-WIKI, TP-WIKI]
allowed_paths: [internal/cli/**, internal/service/**, internal/wikisource/**, internal/docgraph/**, internal/model/**, internal/output/**, .docs/wiki/09_contratos/CT-NAV-WIKI.md, .docs/wiki/06_pruebas/TP-WIKI.md]
forbidden_paths: [internal/service/legacy_pack_contract.go, C:/repos/mios/mi-pi/**, .docs/raw/**, .mi-lsp/**]
verify: [go test ./internal/cli/... ./internal/service/... ./internal/wikisource/... ./internal/docgraph/...]
stop_if: [nav.pack legacy behavior changes, governed corpus cannot be identified deterministically]
secret_scan: {required: true, evidence: names-only-no-values}
---
# Task T1: Wiki pack y validadores gobernados

## Shared Context
**Goal:** G1, eliminar silencio/timeout y falsos positivos wiki.
**Stack:** Go CLI + docgraph/wiki source.
**Architecture:** `nav.wiki.pack` es directo; `nav.pack` legacy conserva routing actual. Validators sólo abren corpus gobernado y prefieren paths canónicos.

## Locked Decisions
- Cambiar únicamente wiki pack de `nav.pack` a `nav.wiki.pack`.
- Excluir `.docs/auditoria/**` y `.docs/raw/**`; `validate-source` exige `SDD-WIKI-SOURCE-v1`.

## Task Metadata
```yaml
id: T1
depends_on: []
agent_type: claudex-writer
goal_id: G1
github_issues: []
expected_outcome: "wiki pack termina con envelope no vacío/error estructurado y validators usan canon gobernado."
files:
  - modify: internal/cli/nav.go:newNavWikiCommand
  - modify: internal/cli/root.go:shouldUseDaemon
  - modify: internal/docgraph/**
  - modify: internal/wikisource/**
  - modify: internal/service/**wiki**
  - modify: internal/cli/**/*test.go
complexity: high
done_when:
  - "focused Go tests exit 0"
  - "validate-source accepts --paths and --ids"
evidence_expected:
  - "operation ID routing assertion"
  - "canonical duplicate-ID winner assertion"
stop_if:
  - "nav.pack contract must change"
  - "validator scope requires regex waiver instead of authority rules"
```

## Reference
`internal/cli/nav.go::newNavWikiCommand` — emit `nav.wiki.pack`; `internal/cli/root.go::shouldUseDaemon` — preserve direct reservation.

## Prompt
Implementa TDD RED/GREEN para operation ID, terminal output, corpus filtering, canonical precedence y filtros simétricos. El selector canónico debe provenir del read model/governed roots, no de una lista de exclusiones arbitraria. Un resultado vacío debe incluir `hint` o error tipado; nunca silencio.

## Execution Procedure
1. Usa `mi-lsp nav search "newNavWikiCommand" --workspace milsp-harness-first --format toon --include-content` y abre sólo los rangos citados.
2. Añade tests que fallen con el operation ID actual y con Markdown de auditoría/raw.
3. Cambia wiki pack a `nav.wiki.pack`; no edites la rama `nav.pack` legacy.
4. Implementa corpus gobernado, deduplicación canónica y flags `--paths`/`--ids` para ambos validators.
5. Ejecuta la suite focalizada y un smoke con binario `go run ./cmd/mi-lsp nav wiki pack` con timeout acotado.

## Skeleton
```go
runWikiPack(operation: "nav.wiki.pack")
validateSource(pathsFilter, idsFilter, governedSourceOnly)
```

## Test
Añadir casos: operation exacta; legacy intacto; auditoría/raw excluidos; duplicate `doc_id` elige canon; ambos nombres de protocolo; filtros paths/ids; envelope vacío con hint.

## Verify
`go test ./internal/cli/... ./internal/service/... ./internal/wikisource/... ./internal/docgraph/...` -> PASS

## Commit
`fix(wiki): route pack directly and validate governed canon - Gabriel Paz -`
