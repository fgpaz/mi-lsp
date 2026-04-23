# Wave 1 - Docs ranking slice

## Ownership

- `.docs/wiki/03_FL/FL-IDX-01.md`
- `.docs/wiki/04_RF/RF-QRY-010.md`
- `.docs/wiki/06_pruebas/TP-QRY.md`
- `.docs/wiki/07_baseline_tecnica.md`
- `.docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md`
- `.docs/wiki/09_contratos/CT-NAV-ASK.md`
- `.docs/wiki/09_contratos_tecnicos.md`
- `internal/docgraph/docgraph.go`
- `internal/service/doc_ranking.go`
- `internal/service/owner_ranking_test.go`

## Required outcomes

- preserve short canonical tokens in tokenization:
  - `RF`
  - `FL`
  - `TP`
  - `CT`
  - `DB`
  - `API`
  - `SDK`
  - `UX`
  - `UI`
  - `OIDC`
- keep `.docs/raw/*` from winning `primary_doc` when a positive `.docs/wiki/*` candidate exists
- treat impact as shared scorer/tokenizer behavior for:
  - `nav ask`
  - `nav route`
  - `nav pack`
  - `nav intent`
- document and test the retained legacy root fallback `.docs/wiki/RF.md` if code keeps it

## Stop condition

If completion requires edits outside the owned paths above, stop and re-scope instead of spilling the slice.
