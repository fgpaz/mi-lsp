---
id: RF-GPH-001
title: Derivar identidad durable y cross-RID de nodos
status: planned
flows:
  - FL-GPH-01
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-001"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-01]]'
  - '[[05_modelo_datos]]'
exports:
  - 'RF-GPH-001'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-01.md
  - .docs/wiki/05_modelo_datos.md
  - .docs/wiki/04_RF/RF-GPH-001.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-001.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-001.md
```

# RF-GPH-001 - Derivar identidad durable y cross-RID de nodos

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| Actores | Indexer, backend semantico, Graph Kernel |
| Prioridad / severidad | critica / critica |
| FL origen | FL-GPH-01 |
| Estado | planned; bloquea RF-GPH-002..011 |

## 2. Resultado requerido

El Graph Kernel debe derivar un `NodeKey` determinista, durable y relocatable para cada nodo publicable. `node_id INTEGER` puede existir como surrogate SQLite local, pero nunca identifica un nodo fuera de una base o generacion.

## 3. Entradas obligatorias de NodeKey v1

La tupla canonica ordenada es:

1. `repository_identity`: identidad estable y sanitizada del repositorio; se resuelve desde un ID explicito del proyecto o desde el origin VCS canonico sin credenciales. Un nombre de carpeta o root absoluto no es suficiente.
2. `backend_type`: backend que aporta la identidad (`roslyn`, `go`, `tsserver`, `pyright` u otro registrado).
3. `language`: identificador de lenguaje normalizado en minusculas ASCII.
4. `project_or_module`: proyecto, assembly, package o modulo estable relativo al repositorio.
5. `owner_path`: path de declaracion relativo al repositorio.
6. `symbol_kind`: clase canonica de nodo (`workspace`, `repository`, `project`, `package`, `file`, `namespace`, `type`, `method`, `function`, `field`, `property`, `event`, `route`, `test`, `document` u otra registrada).
7. `semantic_identity`: identidad estable emitida por compiler/LSP/AST, por ejemplo documentation ID + project key en Roslyn o import path + receiver + nombre + firma normalizada en Go.

El root absoluto, timestamps, orden de enumeracion, separadores del SO y rangos de linea/columna quedan fuera de la identidad. Los rangos pertenecen a `GraphEvidence`.

## 4. Normalizacion y serializacion

- Strings en UTF-8 y Unicode NFC; no se hace case-fold de nombres de simbolo.
- `owner_path` y paths de proyecto usan `/`, son relativos, eliminan `.` y rechazan path absoluto, `..`, NUL o segmentos vacios inesperados.
- Hosts VCS se normalizan en minusculas, se elimina `.git` final y se descartan userinfo, token, query y fragment. Si no puede obtenerse una identidad sin secretos, el nodo queda unresolved.
- Valores enum se serializan con su nombre canonico en minusculas ASCII.
- La serializacion binaria comienza con magic `MILSP-NK`, version `0x01` y cantidad de campos `uint16` big-endian. Cada campo usa `tag uint8 + length uint32 big-endian + value bytes`; los tags siguen el orden de la seccion 3.
- `node_key` es exactamente `SHA-256(payload_canonico)` y se persiste como BLOB de 32 bytes. La representacion externa es lowercase hex de 64 caracteres.
- `cross_rid` de nodo es `milsp:gph-node:v1:<lowercase-hex-node-key>`; no depende del RID del binario ni del host.

## 5. Proceso

1. Resolver y sanitizar `repository_identity`.
2. Solicitar al backend la identidad semantica y su provenance.
3. Normalizar todos los campos con reglas v1.
4. Serializar, calcular SHA-256 y construir el cross-RID.
5. Comparar la tupla canonica contra cualquier fila que ya use el digest.
6. Publicar el nodo solo si identidad, provenance y cross-RID son completos.

## 6. Invariantes

- La misma entrada canonica produce bytes, hash y cross-RID identicos en Windows, Linux y macOS.
- Un cambio solo de root absoluto, line ending o rango de evidencia no cambia `NodeKey`.
- Dos tuplas distintas nunca se fusionan porque compartan display name.
- Si el mismo digest aparece con una tupla diferente, se registra `GraphUnresolved` y se bloquea la generacion completa; no se rehasha con sal aleatoria.
- Un simbolo anonimo/local sin identidad estable queda unresolved hasta que el backend provea un anchor estructural versionado; no se usa `line:column` como sustituto silencioso.
- Toda version futura usa otro magic/version; nunca cambia las reglas de v1 in-place.

## 7. Salida

| Campo | Tipo | Regla |
|---|---|---|
| `node_key` | BLOB(32) | SHA-256 del payload v1 |
| `identity_schema` | string | `milsp-node-key/v1` |
| `identity_fields` | record | campos canonicos comparables para detectar colision |
| `cross_rid` | string | formato v1 estable |
| `provenance` | record | backend, version y source fingerprint |

## 8. Errores tipados

| Codigo | Causa | Resultado |
|---|---|---|
| `GPH_IDENTITY_REPOSITORY_MISSING` | no existe identidad de repo sanitizable | unresolved; no publicar |
| `GPH_IDENTITY_FIELD_MISSING` | falta un campo obligatorio | unresolved; no publicar |
| `GPH_IDENTITY_PATH_INVALID` | path absoluto, traversal o normalizacion invalida | reject |
| `GPH_IDENTITY_BACKEND_UNSTABLE` | backend no ofrece identidad versionada | omission/unresolved |
| `GPH_IDENTITY_COLLISION` | mismo SHA-256 con tupla distinta | generacion invalid |
| `GPH_IDENTITY_VERSION_UNSUPPORTED` | schema no soportado | reject sin fallback |

## 9. Aceptacion y trazabilidad

- Golden vectors byte a byte para la serializacion y SHA-256.
- 30 repeticiones por fixture y validacion cross-RID byte-identical entre RIDs soportados.
- Casos de relocation, Unicode, separadores, case, missing field y colision simulada.
- `TP-GPH / TP-GPH-001 / TC-GPH-001..006`.
