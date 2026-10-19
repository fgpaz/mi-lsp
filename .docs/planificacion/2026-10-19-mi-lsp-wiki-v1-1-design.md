# Diseño wiki v1.1 para mi-lsp

- **Estado del documento:** propuesta de diseño publicada; no constituye evidencia de implementación. Implementación autorizada en v0.14.0.
- **Base:** CLI `v0.13.3`, tag `v0.13.3` → `f871dcff5bb119fc5f41b06f53a41911a9d031a3`. El diseño previo partió de `0c60177eca3d5e248054b69e8d989c25053017a4`; la implementación parte de `origin/main` en `ff0d7275554bc738f9d766fe3c4f7f7797190b43`. Modelo objetivo: wiki v1.1.
- **Interfaces actuales consultadas:** `CT-GRAPH-CLI`, `CT-NAV-WIKI`, `CT-Q-V1`, `RF-GPH-007`; `GraphImpactRequest`, `WikiCodeContext`, `newNavWikiCommand` y `newProviderManifest`. El CLI ya tiene consultas de grafo y una integración wiki↔código parcial. `q-v1` está congelado; el manifiesto mi-lsp actual declara `q:q-v1`, no una API wiki-graph. Por eso la sintaxis nueva abajo es **propuesta**, no interfaz disponible.
- **Alineación:** `persona:wiki-links-en-grafo-unico-milsp-20261019`, `persona:wiki-diagramas-c4-secuencia-y-derivados-20261019`, `persona:wiki-idioma-en-governance-20261019`, `persona:roadmap-afuera-con-capa-fina-20261019`. `mi-decidir consultar` terminó en `backend_timeout`; no se reintentó y no bloquea este diseño.

## 1. Un grafo wiki+código

Incorporar los enlaces tipados `fulfills`, `implements`, `verifies`, `affects`, `supports`, `materializes`, `supersedes`, `experiences`, `stages`, `maps`, `grounds_in` y `code` a la generación graph-native publicada por workspace. `code` enlaza un documento con un archivo y, si está declarado/resuelto, un símbolo del catálogo. Reutilizar NodeKeys/identidades existentes; no crear almacén, grafo paralelo ni overlay persistido. Las aristas inversas se derivan en la consulta, nunca se duplican como enlaces de autoría. Separar provenance `declared`, `observed`, `derived` y `candidate`.

`neighbors`, `path`, `explain`, `affected` y `change-pack` consultan esa misma generación; `trace` pasa a leerla o queda como proyección compatible. Comportamiento esperado: `neighbors(RF)` alcanza FL, TP, ADR, TECH y código declarado; `path(RS, símbolo)` recorre la cadena hasta código; un diff alcanza RF/TP/TECH enlazados. La respuesta conserva frescura, omisiones y provenance; no presenta una generación stale como actual. `WikiCodeContext` sigue siendo una vista docs-first compatible, no otra fuente de aristas.

## 2. API pequeña y paso por mi-mcp

Propuesta: dos operaciones `read` en el `provider-manifest` normal de mi-lsp: `wiki_graph` (`wiki-graph/v1`, CLI `nav wiki graph`) y `wiki_validate_contract` (`wiki-contract/v1`, CLI `nav wiki validate-contract`). No modificar `q-v1` ni añadir lógica wiki, SDK o herramienta de dominio a mi-mcp. Entrada graph tipada: `{workspace, query:{mode:neighbors|path|explain|affected|change-pack|matrix, from?, to?, changed_paths?, ref?}, relations?, direction?, depth?, limit?, cursor?, render?:"mermaid"}`. Salida graph JSON: `{contract_version, workspace, generation_id, doc_generation_id, code_generation_id, graph_freshness, nodes:[{id,kind,doc_id?,path?,symbol?}], edges:[{from,relation,to,provenance}], matrix?:[{rf_id,tp_ids[]}], roadmap?:{canonical:false,entries:[{rs_id,horizon,status}]}, mermaid?:{format:"mermaid",source,generation_id,node_ids[],edge_ids[]}, unresolved, warnings, truncated, continuation:{next_cursor?}}`. `roadmap` solo aparece si existe la fuente opcional y sus campos son metadata no canónica; no crea aristas. `mermaid` solo aparece cuando se pide el render y queda vinculado a la generación y página consultadas. `wiki_validate_contract` acepta workspace/ids/paths y devuelve `{contract_version,language,checks,warnings}`; sus warnings son datos tipados, no errores de diagrama. El cursor reutiliza el vínculo existente a consulta, workspace, generación, filtros y orden; una generación distinta no se mezcla con la página anterior.

Ejemplo **propuesto**, no disponible hoy, de un bloque Codemode que pagina y filtra/resume en JavaScript:

```js
const edges = []; let cursor;
do {
  const g = await call({cli:"mi-lsp", comando:"nav wiki graph", args:{
    workspace:"<repo>", query:{mode:"path", from:"<RS doc_id>", to:"<q-v1 symbol id>"}, limit:50, cursor
  }});
  edges.push(...g.edges);
  cursor = g.continuation?.next_cursor;
} while (cursor);
return edges.filter(e => e.provenance !== "candidate").slice(0,8)
  .map(({from,relation,to}) => ({from,relation,to}));
```

## 3. Mermaid derivado, no canónico

`render:"mermaid"` serializa bajo demanda la misma página de grafo (RS→FL→RF→TP→code o vecinos), para OpenUI/artifacts. La salida incluye su `generation_id`; no se guarda en la wiki, no crea artefactos ni otro generador/servicio. Si faltan datos o la generación está stale, se informa la omisión en vez de completar enlaces inventados.

## 4. Idioma, claves y matriz

El nombre canónico de governance es `.docs/wiki/00_governance.md`. La fuente actual inspeccionada contiene `.docs/wiki/00_gobierno_documental.md`; se admite ese nombre como compatibilidad legada y se avisa, sin sustituir el canon ni presumir otros alias. `00_governance.md` declara `language: es|en`. Claves semánticas de sección estables e independientes del título (por ejemplo, `criterion_N`); títulos locales según ese campo, `Criterio N`/`Criterion N`, con la misma clave y numeración. Solo si falta `language`, validar en español y emitir warning; si el valor es inválido, reutilizar la convención tipada existente, sin tratarlo como ausente ni inventar política. No inferir idioma del cuerpo ni reescribirlo. `validate-contract` y la matriz RF×TP leen esas claves; la matriz se deriva de aristas `verifies`, no de una segunda lista de enlaces. Conservar la autoridad textual de wiki sobre el código y los diagramas.

## 5. Diagramas ausentes: avisos, nunca gates

La ausencia de C4 Context/Container en `03_architecture.md` o de secuencia en una FL produce warnings tipados (`missing_c4_context`, `missing_c4_container`, `missing_sequence`) en el resultado; no `stop_if`, estado bloqueante ni exit no-cero por ese motivo. Si diagrama y texto discrepan, manda el texto.

## 6. Roadmap opcional y base de validación

`.docs/roadmap/roadmap.md` es una capa opcional con metadata `rs_id`, `horizon` y `status` junto a RS, explícitamente **no canónica**: no crea aristas, requisitos ni autoridad y no requiere integración externa. La respuesta expone esa metadata solo en `roadmap:{canonical:false,entries:[...]}`; su ausencia no afecta búsquedas ni validación. La segunda `validate-contract` de `v0.13.3` ya está hecha en la base indicada: este diseño la consume, no la repite ni abre otra QA. Las nuevas reglas de idioma/warnings quedan diseñadas, no verificadas en esta entrega.

**Fronteras para repartir después (sin asignar ejecutores aquí):** grafo/API/Mermaid — `internal/wikisource/parser.go`, `internal/model/graph_impact.go`, `internal/model/wiki_code_context.go`, `internal/service/trace.go`, `internal/docgraph/canonical_navigation.go`, `internal/cli/nav.go` y `internal/cli/provider_manifest.go`; idioma/validación — `internal/service/harness_validate.go`, `internal/service/source_validate.go` y el mismo `internal/cli/nav.go`; roadmap — navegación wiki existente. Todo sigue bajo ownership del repo mi-lsp. `nav.go` es seam compartido: un único integrador modifica el registro CLI/manifiesto; Root asigna el trabajo posterior.
