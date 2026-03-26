# TP-WKS

## Cobertura objetivo

- RF-WKS-001
- RF-WKS-002
- RF-WKS-003

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-WKS-001 | positivo | RF-WKS-001 | registra workspace compatible con alias explicito |
| TC-WKS-002 | positivo | RF-WKS-001 | registra workspace con alias derivado del root |
| TC-WKS-003 | negativo | RF-WKS-001 | rechaza path inexistente o layout incompatible sin side effects |
| TC-WKS-004 | positivo | RF-WKS-001 | detecta workspace Python con `pyproject.toml` y reporta `language: python` |
| TC-WKS-005 | positivo | RF-WKS-001 | detecta workspace mixto Python+TS y reporta ambos lenguajes |
| TC-WKS-006 | positivo | RF-WKS-002 | workspace add sin --no-index indexa automaticamente |
| TC-WKS-007 | positivo | RF-WKS-002 | workspace add con --no-index salta indexing |
| TC-WKS-008 | negativo | RF-WKS-002 | workspace add indexa pero falla → warning, registro exitoso |
| TC-WKS-009 | positivo | RF-WKS-003 | `init` registra el repo actual, indexa y devuelve `next_steps` para `nav ask` |
| TC-WKS-010 | negativo | RF-WKS-003 | `init` rechaza un path incompatible sin registro parcial |
