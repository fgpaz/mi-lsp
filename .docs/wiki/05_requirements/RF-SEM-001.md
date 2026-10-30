---
wiki_contract: WIKI-CONTRACT-v1
id: RF-SEM-001
kind: requirement
status: active
title: RF-SEM-001 - Configurar backend de embeddings pluggable OpenAI-compatible por workspace
links:
  implements:
  - FL-SEM-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-SEM-001 - Configurar backend de embeddings pluggable OpenAI-compatible por workspace

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-SEM-001
- **Campo**: Título; **Valor**: Configurar backend de embeddings pluggable OpenAI-compatible por workspace
- **Campo**: Actores; **Valor**: Desarrollador, Skill, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-SEM-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace registrado con `.mi-lsp/project.toml` accesible; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Backend embeddings disponible (OpenAI-compatible); **Tipo**: técnica; **Estado requerido**: opcional
- **Condición**: Perfil knowledge-wiki efectivo; **Tipo**: operativa; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. CLI recibe comando `nav recall` o similar.
2. Core lee `.mi-lsp/project.toml` seccion `[embeddings]` si existe.
3. Si `[embeddings]` no existe, falta `base_url`/`model`, o `enabled=false`, devuelve hint y no llama al backend.
4. Si `base_url` + `model` existen y `enabled` esta omitido o `true`, carga cliente OpenAI-compatible y resuelve `api_key_env` según el contrato de autenticacion.
5. Si `api_key_env` nombra una variable cuyo valor esta ausente, vacio o compuesto solo por whitespace, devuelve `SEM_API_KEY_MISSING` antes de construir o enviar una solicitud de red.
6. Ejecuta embedding del query o de chunks de wiki con timeout `timeout_ms`, payload `encoding_format = "float"` cuando no haya override y headers `Accept: application/json` + `User-Agent`; solo agrega Bearer cuando el valor resuelto esta presente.
7. Si la llamada falla, registra warning y orienta el fallback canónico a `mi-lsp nav wiki search`; no existe fallback local oculto.
8. Config efectiva se resuelve desde `.mi-lsp/project.toml` por operacion.

### 7. Casos especiales y variantes
- Si no hay seccion `[embeddings]` en `project.toml`, default es `embeddings_unconfigured` con hint accionable (no error).
- Si hay `base_url` + `model` y `enabled` esta omitido, la config esta activa.
- Si `enabled=false`, la config queda apagada aunque existan `base_url` + `model`.
- Si `provider` es vacio o `null`, la compatibilidad se decide por `base_url` + `model`; no se habilita un proveedor local implicito.
- Si `api_key_env` es vacio o whitespace, el endpoint OpenAI-compatible se usa sin autenticacion; no se exige una variable por default.
- Si `api_key_env` contiene un nombre y el valor resuelto es ausente, vacio o whitespace, `SEM_API_KEY_MISSING` se determina antes de cualquier I/O de red; el caller degrada con warning a `nav wiki search`.
- Los mensajes de error y warning no incluyen nombres de variables ni valores de credenciales, salvo identificadores publicos canónicos como `SEM_API_KEY_MISSING`.
- Si backend esta accesible pero timeout en validación, registra warning y orienta a `nav wiki search`.
- Config puede ser override parcial por CLI flags (future expansion).
- `encoding_format` omitido se resuelve a `float` para proveedores OpenAI-compatible que lo soportan.
- `user_agent` omitido usa el default seguro del cliente.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir configuración global (`~/.mi-lsp/`); es por workspace
  - no exponer credenciales en output
  - no fallar si backend no disponible
- Decisiones cerradas:
  - fallback documental seguro es `nav wiki search`
  - credenciales via env var, nunca en `project.toml`
- TODO explicit = 0
- Fuera de alcance:
  - certificados SSL custom (future)
  - proxy HTTP (future)

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `enabled`; **Tipo**: bool; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: omitido = activo si `base_url` + `model`; `false` = apagado explícito; **RN**: RF-SEM-001
- **Campo**: `provider`; **Tipo**: string; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: e.g. `openai`, `azure`, `custom`, `nan` o vacio; **RN**: RF-SEM-001
- **Campo**: `base_url`; **Tipo**: string; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: valid HTTP(S) URL o vacio; **RN**: RF-SEM-001
- **Campo**: `model`; **Tipo**: string; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: nombre del modelo OpenAI-compatible o vacio; **RN**: RF-SEM-001
- **Campo**: `dim`; **Tipo**: entero; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: > 0; depende del modelo elegido; **RN**: RF-SEM-001
- **Campo**: `api_key_env`; **Tipo**: string; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: env var name (e.g. `MI_LSP_EMBEDDINGS_API_KEY`) o vacio; **RN**: RF-SEM-001
- **Campo**: `profile`; **Tipo**: enum; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: `spec-driven` o `knowledge-wiki`; default `knowledge-wiki`; **RN**: RF-SEM-001
- **Campo**: `batch_size`; **Tipo**: entero; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: >= 1, default 10; **RN**: RF-SEM-001
- **Campo**: `timeout_ms`; **Tipo**: entero; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: > 0, default 30000; **RN**: RF-SEM-001
- **Campo**: `encoding_format`; **Tipo**: string; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: OpenAI-compatible; default operativo `float`; **RN**: RF-SEM-001
- **Campo**: `user_agent`; **Tipo**: string; **Req.**: no; **Origen**: `.mi-lsp/project.toml [embeddings]`; **Validacion**: header HTTP seguro; default del cliente si se omite; **RN**: RF-SEM-001

Ejemplo operativo generico:

```toml
[embeddings]
provider = "openai"
base_url = "https://embeddings.example.local/v1"
model = "text-embedding-model"
dim = 1536
api_key_env = "MI_LSP_EMBEDDINGS_API_KEY"
profile = "knowledge-wiki"
batch_size = 32
timeout_ms = 30000
encoding_format = "float"
user_agent = "mi-lsp-embeddings/1.0"
```

`api_key_env` nombra una variable de entorno. El valor real de la key nunca va en `project.toml`, logs, wiki, output de CLI ni evidencia; para ejecuciones locales usar env del shell o `mkey run`. Si `api_key_env` es vacio o contiene solo whitespace, el endpoint OpenAI-compatible se trata explicitamente como endpoint sin autenticacion. Si contiene un nombre y el valor resuelto esta ausente, vacio o compuesto solo por whitespace, se devuelve `SEM_API_KEY_MISSING` antes de cualquier I/O de red. Un valor presente se envia unicamente como `Authorization: Bearer <valor>`; los mensajes publicos no incluyen el nombre de la variable ni su valor.

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: interno; **Efecto observable**: configuración cargada exitosamente
- **Campo**: `backend`; **Tipo**: string; **Destino**: interno; **Efecto observable**: `recall` si usa embeddings; fallback documental recomendado: `nav wiki search`
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: diagnóstico de config fallida o fallback activado
- **Campo**: `config_effective`; **Tipo**: objeto; **Destino**: diagnóstico; **Efecto observable**: snapshot de config usado (sin credenciales)

### 8. Impacto en el modelo de datos
- Config almacenado en `EmbeddingsBlock` / cliente embeddings durante la operación
- Credenciales nunca persistidas en disco; siempre via env var

## Errores

### 6. Errores tipados
- **Código**: `SEM_INVALID_CONFIG`; **Causa**: config esta malformada; **Trigger**: `project.toml [embeddings]` inválido; **Respuesta esperada**: warning + fallback a `nav wiki search`
- **Código**: `SEM_PROVIDER_UNREACHABLE`; **Causa**: backend no responde; **Trigger**: timeout o error de red; **Respuesta esperada**: warning + fallback a `nav wiki search`
- **Código**: `SEM_API_KEY_MISSING`; **Causa**: `api_key_env` nombra una variable pero su valor resuelto esta ausente, vacio o compuesto solo por whitespace; **Trigger**: validación local previa a cualquier I/O de red; **Respuesta esperada**: warning + fallback seguro a `nav wiki search`; mensaje sin identificadores ni valores sensibles
- **Código**: `SEM_DIMENSION_MISMATCH`; **Causa**: proveedor responde otra dimensión; **Trigger**: vector recibido no coincide con `dim`; **Respuesta esperada**: error de embedding y fallback documental

## Criterios de aceptación (numerados)

1. Cargar config valida desde project.toml: Dado un workspace con [embeddings] section en project.toml valida Y la config incluye base_url y model Y enabled esta omitido Cuando ejecuto "mi-lsp nav recall 'query' --workspace <alias>" Entonces la config se carga exitosamente Y backend = "recall" Y el payload usa encoding_format = "float" Y la dimensión recibida coincide con dim
2. Apagar embeddings con kill switch explícito: Dado un workspace con [embeddings] section en project.toml valida Y enabled = false Cuando ejecuto "mi-lsp index --workspace <alias> --docs-only" Entonces no se llama al provider de embeddings Y wiki_chunk_embeddings no recibe filas nuevas
3. Caer a offline si backend no responde: Dado config que apunta a un backend no disponible Cuando ejecuto "mi-lsp nav recall 'query' --workspace <alias>" Entonces se registra warning de provider unreachable Y la guidance recomienda `mi-lsp nav wiki search` Y no se usa un fallback local oculto
4. Usar endpoint compatible sin autenticacion explícita: Dado una config activa con `api_key_env` vacio o compuesto solo por whitespace Cuando ejecuto "mi-lsp nav recall 'query' --workspace <alias>" Entonces la solicitud puede dirigirse al endpoint OpenAI-compatible sin header Bearer Y no se exige una variable de entorno por default
5. Rechazar key ausente antes de I/O de red: Dado una config activa cuyo `api_key_env` nombra una variable ausente, vacia o whitespace Cuando ejecuto "mi-lsp nav recall 'query' --workspace <alias>" Entonces devuelve `SEM_API_KEY_MISSING` antes de cualquier I/O de red Y el mensaje no incluye nombre ni valor sensible Y el caller degrada con warning a `mi-lsp nav wiki search`
6. Usar Bearer con key presente: Dado una config activa cuyo `api_key_env` resuelve a un valor no vacio Cuando ejecuto "mi-lsp nav recall 'query' --workspace <alias>" Entonces la solicitud usa `Authorization: Bearer <valor>`
7. Usar default offline si no hay config: Dado un workspace sin seccion [embeddings] Cuando ejecuto "mi-lsp nav recall 'query' --workspace <alias>" Entonces no hay error, backend = "recall" Y items = [] Y hint menciona embeddings Y no se intenta remote

### 9. Escenarios de aceptación (Gherkin)
```gherkin
Scenario: Cargar config valida desde project.toml
  Given un workspace con [embeddings] section en project.toml valida
  And la config incluye base_url y model
  And enabled esta omitido
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then la config se carga exitosamente
  And backend = "recall"
  And el payload usa encoding_format = "float"
  And la dimension recibida coincide con dim

Scenario: Apagar embeddings con kill switch explicito
  Given un workspace con [embeddings] section en project.toml valida
  And enabled = false
  When ejecuto "mi-lsp index --workspace <alias> --docs-only"
  Then no se llama al provider de embeddings
  And wiki_chunk_embeddings no recibe filas nuevas

Scenario: Caer a offline si backend no responde
  Given config que apunta a un backend no disponible
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then se registra warning de provider unreachable
  And la guidance recomienda `mi-lsp nav wiki search`
  And no se usa un fallback local oculto

Scenario: Usar endpoint compatible sin autenticacion explicita
  Given una config activa con `api_key_env` vacio o compuesto solo por whitespace
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then la solicitud puede dirigirse al endpoint OpenAI-compatible sin header Bearer
  And no se exige una variable de entorno por default

Scenario: Rechazar key ausente antes de I/O de red
  Given una config activa cuyo `api_key_env` nombra una variable ausente, vacia o whitespace
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then devuelve `SEM_API_KEY_MISSING` antes de cualquier I/O de red
  And el mensaje no incluye nombre ni valor sensible
  And el caller degrada con warning a `mi-lsp nav wiki search`

Scenario: Usar Bearer con key presente
  Given una config activa cuyo `api_key_env` resuelve a un valor no vacio
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then la solicitud usa `Authorization: Bearer <valor>`

Scenario: Usar default offline si no hay config
  Given un workspace sin seccion [embeddings]
  When ejecuto "mi-lsp nav recall 'query' --workspace <alias>"
  Then no hay error, backend = "recall"
  And items = []
  And hint menciona embeddings
  And no se intenta remote
```

### 10. Trazabilidad de pruebas
- Positivo: `TP-SEM / TC-SEM-001`
- Positivo: `TP-SEM / TC-SEM-002`
- Negativo: `TP-SEM / TC-SEM-003`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-SEM-001
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-SEM-01.md
exports:
- RF-SEM-001
- RF-SEM-001 - Configurar backend de embeddings pluggable OpenAI-compatible por workspace
agent_must_read:
- .docs/wiki/05_requirements/RF-SEM-001.md
- .docs/wiki/04_flows/FL-SEM-01.md
- .docs/wiki/06_tests/TP-SEM.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-SEM-001.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-SEM.md sin inferir que
  fueron ejecutados.
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-SEM-001.md
- .docs/wiki/06_tests/TP-SEM.md
```
