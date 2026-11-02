---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-001
kind: requirement
status: draft
title: RF-GPH-001 - Derivar identidad durable y cross-RID de nodos
links:
  implements:
  - FL-GPH-01
  code:
  - path: internal/indexer/extractor_kotlin.go
    role: behavior
  - path: internal/model/graph.go
    role: behavior
  - path: internal/model/graph_observation.go
    role: behavior
---

## Propósito

RF-GPH-001 - Derivar identidad durable y cross-RID de nodos

### 1. Hoja de ejecución
- **Campo**: Actores; **Valor**: Indexer, backend semántico, Graph Kernel
- **Campo**: Prioridad / severidad; **Valor**: critica / critica
- **Campo**: FL origen; **Valor**: FL-GPH-01
- **Campo**: Estado; **Valor**: planned; bloquea RF-GPH-002..011

## Comportamiento ordenado

### 2. Resultado requerido

El Graph Kernel debe derivar un `NodeKey` determinista, durable y relocatable para cada nodo publicable. `node_id INTEGER` puede existir como surrogate SQLite local, pero nunca identifica un nodo fuera de una base o generacion.

### 4. Normalización y serializacion

- Strings en UTF-8 y Unicode NFC; no se hace case-fold de nombres de simbolo.
- `owner_path` y paths de proyecto usan `/`, son relativos, eliminan `.` y rechazan path absoluto, `..`, NUL o segmentos vacios inesperados.
- Hosts VCS se normalizan en minusculas, se elimina `.git` final y se descartan userinfo, token, query y fragment. Si no puede obtenerse una identidad sin secretos, el nodo queda unresolved.
- Valores enum se serializan con su nombre canónico en minusculas ASCII.
- La serializacion binaria comienza con magic `MILSP-NK`, version `0x01` y cantidad de campos `uint16` big-endian. Cada campo usa `tag uint8 + length uint32 big-endian + value bytes`; los tags siguen el orden de la seccion 3.
- `node_key` es exactamente `SHA-256(payload_canonico)` y se persiste como BLOB de 32 bytes. La representacion externa es lowercase hex de 64 caracteres.
- `cross_rid` de nodo es `milsp:gph-node:v1:<lowercase-hex-node-key>`; no depende del RID del binario ni del host.

### 5. Proceso

1. Resolver y sanitizar `repository_identity`.
2. Solicitar al backend la identidad semántica y su provenance.
3. Normalizar todos los campos con reglas v1.
4. Serializar, calcular SHA-256 y construir el cross-RID.
5. Comparar la tupla canónica contra cualquier fila que ya use el digest.
6. Publicar el nodo solo si identidad, provenance y cross-RID son completos.

### 6. Invariantes

- La misma entrada canónica produce bytes, hash y cross-RID identicos en Windows, Linux y macOS.
- Un cambio solo de root absoluto, line ending o rango de evidencia no cambia `NodeKey`.
- Dos tuplas distintas nunca se fusionan porque compartan display name.
- Si el mismo digest aparece con una tupla diferente, se registra `GraphUnresolved` y se bloquea la generación completa; no se rehasha con sal aleatoria.
- Un símbolo anonimo/local sin identidad estable queda unresolved hasta que el backend provea un anchor estructural versionado; no se usa `line:column` como sustituto silencioso.
- Toda version futura usa otro magic/version; nunca cambia las reglas de v1 in-place.

### Admisión Kotlin aditiva

- **Marcador estable**: RF-GPH-001.kotlin-admission-v1
- **owner**: coord-milsp
- **bindings**:
  - internal/model/graph.go
  - internal/model/graph_observation.go
  - internal/indexer/extractor_kotlin.go
- **Estado del comportamiento**: experimental_unverified_explicit_incomplete
- **Comprobación prevista**:
  - .docs/wiki/06_tests/TP-GPH.md::TP-GPH.kotlin-compiler-export-v1

[[ADR-KOTLIN-GRAPH-ADMISSION-V1]] registra exactamente `kotlin-language-server` / `kotlin` / `kotlin/binding-context` para declarations nombradas y references compiler locales main-jvm. Decisión delegada coord-milsp, no promoción ni aprobación humana/global inferida. NodeKey-v1/framing/tablas/ReadyForStaging intactos; sin DDL/querybackfill/migración en query. Lectores viejos rechazan el enum nuevo; mínimo lector con SHA/versión de primera release todavía pendiente. Rollback conserva prior generation y binario compatible. Raw partial/experimental actual no publica; autoridad Gradle y conformance pendientes coord-milsp, 2026-10-17.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas obligatorias de NodeKey v1

La tupla canónica ordenada es:

1. `repository_identity`: identidad estable y sanitizada del repositorio; se resuelve desde un ID explícito del proyecto o desde el origin VCS canónico sin credenciales. Un nombre de carpeta o root absoluto no es suficiente.
2. `backend_type`: backend que aporta la identidad (`roslyn`, `go`, `tsserver`, `pyright` u otro registrado).
3. `language`: identificador de lenguaje normalizado en minusculas ASCII.
4. `project_or_module`: proyecto, assembly, package o módulo estable relativo al repositorio.
5. `owner_path`: path de declaracion relativo al repositorio.
6. `symbol_kind`: clase canónica de nodo (`workspace`, `repository`, `project`, `package`, `file`, `namespace`, `type`, `method`, `function`, `field`, `property`, `event`, `route`, `test`, `document` u otra registrada).
7. `semantic_identity`: identidad estable emitida por compiler/LSP/AST, por ejemplo documentation ID + project key en Roslyn o import path + receiver + nombre + firma normalizada en Go.

El root absoluto, timestamps, orden de enumeracion, separadores del SO y rangos de linea/columna quedan fuera de la identidad. Los rangos pertenecen a `GraphEvidence`.

### 7. Salida

- **Campo**: `node_key`; **Tipo**: BLOB(32); **Regla**: SHA-256 del payload v1
- **Campo**: `identity_schema`; **Tipo**: string; **Regla**: `milsp-node-key/v1`
- **Campo**: `identity_fields`; **Tipo**: record; **Regla**: campos canónicos comparables para detectar colision
- **Campo**: `cross_rid`; **Tipo**: string; **Regla**: formato v1 estable
- **Campo**: `provenance`; **Tipo**: record; **Regla**: backend, version y source fingerprint

## Errores

### 8. Errores tipados

- **Código**: `GPH_IDENTITY_REPOSITORY_MISSING`; **Causa**: no existe identidad de repo sanitizable; **Resultado**: unresolved; no publicar
- **Código**: `GPH_IDENTITY_FIELD_MISSING`; **Causa**: falta un campo obligatorio; **Resultado**: unresolved; no publicar
- **Código**: `GPH_IDENTITY_PATH_INVALID`; **Causa**: path absoluto, traversal o normalización inválida; **Resultado**: reject
- **Código**: `GPH_IDENTITY_BACKEND_UNSTABLE`; **Causa**: backend no ofrece identidad versionada; **Resultado**: omission/unresolved
- **Código**: `GPH_IDENTITY_COLLISION`; **Causa**: mismo SHA-256 con tupla distinta; **Resultado**: generación invalid
- **Código**: `GPH_IDENTITY_VERSION_UNSUPPORTED`; **Causa**: schema no soportado; **Resultado**: reject sin fallback

## Criterios de aceptación (numerados)

1. Golden vectors byte a byte para la serializacion y SHA-256.
2. 30 repeticiones por fixture y validación cross-RID byte-identical entre RIDs soportados.
3. Casos de relocation, Unicode, separadores, case, missing field y colision simulada.

### 9. Aceptacion y trazabilidad

- Golden vectors byte a byte para la serializacion y SHA-256.
- 30 repeticiones por fixture y validación cross-RID byte-identical entre RIDs soportados.
- Casos de relocation, Unicode, separadores, case, missing field y colision simulada.
- `TP-GPH / TP-GPH-001 / TC-GPH-001..006`.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-GPH-001
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-GPH-01.md
exports:
- RF-GPH-001
- RF-GPH-001 - Derivar identidad durable y cross-RID de nodos
agent_must_read:
- .docs/wiki/05_requirements/RF-GPH-001.md
- .docs/wiki/04_flows/FL-GPH-01.md
- .docs/wiki/06_tests/TP-GPH.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-GPH-001.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/indexer/extractor_kotlin.go
- internal/model/graph.go
- internal/model/graph_observation.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-GPH.md, ejecutar el
  comando publicado: go test ./internal/model/... ./internal/indexer/... ./internal/service/...
  ./internal/daemon/... (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-GPH-001.
evidence:
- .docs/wiki/05_requirements/RF-GPH-001.md
- .docs/wiki/06_tests/TP-GPH.md
```
