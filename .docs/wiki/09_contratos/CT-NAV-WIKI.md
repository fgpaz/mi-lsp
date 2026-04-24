# CT-NAV-WIKI

## Invocacion

```
mi-lsp nav wiki search <query> --workspace <alias> [--layer RS,RF,FL,TP,CT,TECH,DB] [--top N] [--offset N] [--include-content] [--format toon|compact|yaml]
mi-lsp nav wiki route <task> --workspace <alias> [--full] [--format toon|compact|yaml]
mi-lsp nav wiki pack <task> --workspace <alias> [--rf RF-*] [--fl FL-*] [--doc <path>] [--full] [--format toon|compact|yaml]
mi-lsp nav wiki trace <DOC-ID|--all> --workspace <alias> [--summary] [--format toon|compact|yaml]
```

## Semantica

`nav wiki` es la puerta documental explicita para agentes. `wiki search` usa el docgraph repo-local y el scorer owner-aware para devolver candidatos wiki, mientras `wiki route`, `wiki pack` y `wiki trace` reutilizan la semantica y el shape de `nav route`, `nav pack` y `nav trace`. `wiki search` acepta `RS` como layer outcome y `wiki trace` acepta `RS-*`, `RF-*` y `TP-*`; `--all` sigue recorriendo el set RF canonico, y cuando necesita fallback a disco debe priorizar las rutas gobernadas por `00`/`read-model` antes de caer a layouts legacy.

## Envelope `wiki search`

```json
{
  "ok": true,
  "backend": "wiki.search",
  "workspace": "alias",
  "items": [
    {
      "doc_id": "RF-QRY-016",
      "path": ".docs/wiki/04_RF/RF-QRY-016.md",
      "title": "RF-QRY-016 - Explorar la wiki con una superficie dedicada para agentes",
      "layer": "RF",
      "family": "functional",
      "stage": "requirements",
      "score": 120,
      "why": ["doc_id=RF-QRY-016", "canonical_match"],
      "next_queries": [
        "mi-lsp nav wiki pack \"wiki agentes\" --workspace alias --doc .docs/wiki/04_RF/RF-QRY-016.md --format toon",
        "mi-lsp nav wiki trace RF-QRY-016 --workspace alias --format toon",
        "mi-lsp nav multi-read .docs/wiki/04_RF/RF-QRY-016.md:1-120 --workspace alias --format toon"
      ]
    }
  ],
  "warnings": [],
  "stats": {"files": 1},
  "truncated": false
}
```

## Filtros de capa

| Layer | Docs incluidos |
|---|---|
| `RS` | `02_resultados_soluciones_usuario.md`, `02_resultados/*`, `doc_id=RS-*` |
| `FL` | `03_FL*`, `doc_id=FL-*` |
| `RF` | `04_RF*`, `doc_id=RF-*` |
| `TP` | `06_pruebas*`, `doc_id=TP-*` |
| `TECH` | `07_*`, `07_tech/*`, `doc_id=TECH-*` |
| `DB` | `08_*`, `08_db/*`, `doc_id=DB-*` |
| `CT` | `09_*`, `09_contratos/*`, `doc_id=CT-*` |

## Diagnosticos

- Si `governance_blocked=true`, `wiki search` devuelve `backend=governance` y no ejecuta ranking documental.
- Si `doc_records` esta vacio, `wiki search` devuelve `backend=wiki.search`, `items=[]` y un hint hacia `mi-lsp index --workspace <alias> --docs-only`.
- Si `--layer` contiene valores desconocidos, se ignoran y se devuelven warnings con los layers validos.
- `--repo` no pertenece a `nav wiki`; para compatibilidad, `nav ask|route|pack --repo <x>` lo acepta, lo ignora para docs y sugiere `nav wiki`.
- `nav wiki trace RS-*` devuelve identidad documental (`doc_id`, `layer=RS`, `stage=outcome`) y no rellena el campo legacy `rf`; `nav wiki trace --all` permanece RF-only.

## Estado

implemented

## RF asociado

RF-QRY-016
