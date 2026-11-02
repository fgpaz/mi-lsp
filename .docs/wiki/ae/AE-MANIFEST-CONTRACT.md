# Contrato de lectura del manifiesto AE de compatibilidad

Este contrato describe únicamente `.docs/wiki/ae/AE-MANIFEST.toml`: índice histórico de carga diferida de los nueve documentos AE de compatibilidad. No reemplaza al Kernel v2 externo, no cubre otros documentos sin contrato y no transforma documentos WIKI en Harness.

El TOML conserva su formato consumible. Su identificador indexado `AE-MANIFEST` se exporta aquí para vincular el manifiesto con este contrato explícito, no como excepción del validador. Los campos `sha256` vacíos no acreditan integridad ni certificación; este documento no los completa con valores supuestos. Leer o validar las rutas del manifiesto no equivale a ejecutar sus verificaciones ni a aprobar una release.

```yaml
harness_protocol: SDD-HARNESS-v1
id: AE-MANIFEST-CONTRACT
kind: support-doc
audience: llm-first
imports:
  - AE-PROJECTION-POLICY
  - AE-HARNESS-MANIFEST
exports:
  - AE-MANIFEST-CONTRACT
  - AE-MANIFEST
agent_must_read:
  - .docs/wiki/ae/AE-MANIFEST.toml
  - .docs/wiki/ae/AE-PROJECTION-POLICY.md
  - .docs/wiki/ae/AE-HARNESS-MANIFEST.md
agent_may_edit:
  - .docs/wiki/ae/AE-MANIFEST.toml
  - .docs/wiki/ae/AE-MANIFEST-CONTRACT.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
  - .docs/ae/repo-policy.yaml
verify:
  - >-
    python3 -c "import hashlib,pathlib,re,tomllib; p=pathlib.Path('.docs/wiki/ae/AE-MANIFEST.toml'); d=tomllib.loads(p.read_text()); docs=d['docs']; assert len(docs)==9 and '.docs/wiki/ae/AE-MANIFEST-CONTRACT.md' not in {x['path'] for x in docs}; assert all(pathlib.Path(x['path']).is_file() for x in docs); assert all(not x['sha256'] or (re.fullmatch(r'[0-9a-f]{64}',x['sha256']) and hashlib.sha256(pathlib.Path(x['path']).read_bytes()).hexdigest()==x['sha256']) for x in docs)"
  - "Lectura/cotejo: contrastar que el TOML siga siendo un índice histórico de compatibilidad y que la autoridad permanece en Kernel v2 con .docs/ae/repo-policy.yaml y AE-PROJECTION-POLICY; SHA vacío no acredita integridad, release ni ejecución."
stop_if:
  - Falta algún documento referenciado o el TOML no puede parsearse.
  - La propuesta atribuye autoridad universal a este índice de compatibilidad.
  - Se intenta declarar certificación o ejecución a partir de un hash vacío o de una referencia documental.
evidence:
  - .docs/wiki/ae/AE-MANIFEST.toml
  - .docs/wiki/ae/AE-PROJECTION-POLICY.md
  - .docs/ae/repo-policy.yaml
```
