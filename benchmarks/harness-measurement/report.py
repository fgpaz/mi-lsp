#!/usr/bin/env python3
"""Offline rendering only. Never launches models, diagnostics, tests, or product code."""
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
DATA=ROOT/'results/harness-measurement'
MODELS=('minimax','longcat')

def load(phase,model):
    p=DATA/phase/model/'metrics.json'
    return json.loads(p.read_text()) if p.exists() else None

def val(row,key):
    if row is None: return None
    if key.startswith('usage.'):
        return row['usage'][key.split('.')[1]]['value']
    if key=='successes': return row['oracle']['successes']
    if key=='detected_before_test': return row['oracle']['detected_before_test']
    return row.get(key)

def display(v):
    return 'unknown' if v is None else str(v)

def comparable(before,after):
    if not before or not after: return 'pending_final_verify'
    keys=('harness','model_requested','prompt_sha256','guard_sha256','runner_sha256','fixture_sha256','limits','cwd')
    changed=[k for k in keys if before.get(k)!=after.get(k)]
    if changed: return 'not_comparable: changed_'+','.join(changed)
    if before['status']!='completed' or after['status']!='completed':
        return 'not_comparable: '+before['status']+'/'+after['status']
    return 'paired_alias_only_revision_unknown'

def main():
    lines=['# Medición baseline y receta after','',
           'Estado: baseline medido; after pendiente de producto integrado y FINAL_VERIFY del padre. '
           'No es aceptación de implementación. Un intento por modelo, sin reintentos de campaña.','',
           '## Datos crudos por conversación','',
           '| Modelo | Fase | Estado literal | Input | Output | Cache read | Cache write | Total | Peticiones observadas | Respuestas | Tools | Errores tool | Archivos read | Aciertos / 3 | Error detectado / 1 | Segundos |',
           '|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|']
    cols=('usage.input','usage.output','usage.cacheRead','usage.cacheWrite','usage.totalTokens','request_payloads','assistant_responses','tool_calls','tool_errors','explicit_files_read_count','successes','detected_before_test','seconds')
    rows={}
    for model in MODELS:
        for phase in ('baseline','after'):
            row=load(phase,model); rows[(phase,model)]=row
            state=row['status'] if row else 'pending_final_verify'
            lines.append('| '+ ' | '.join([model,phase,state]+[display(val(row,k)) for k in cols])+' |')
    lines+=['','Los tokens son los contadores reales de `message_end.message.usage` de Pi, no tamaño de archivos. '
             'Unknown conserva motivo en metrics.json. Ceros de caché son normalización del harness, no evidencia independiente de ausencia de caché. '
             'En intentos con errores, la suma de respuestas completadas no es un total facturado exhaustivo. '
             'Los archivos son lecturas explícitas distintas; lecturas implícitas del backend: unknown.','',
             'El estado `harness_error` es conservador: el recolector trata cualquier línea de stderr como incidencia; '
             'su cuerpo se descartó al sanitizar. Aunque exit 0, HTTP 200 y los oráculos completen, esa línea no puede '
             'reclasificarse como inocua sin evidencia. Se conserva como incidencia desconocida, no se repite la corrida.','',
             '## Ratios after / baseline por modelo','',
             '| Modelo | Métrica | Ratio / motivo |','|---|---|---|']
    for model in MODELS:
        before,after=rows[('baseline',model)],rows[('after',model)]
        state=comparable(before,after)
        for k in cols:
            b,a=val(before,k),val(after,k)
            ratio=state if state!='paired_alias_only_revision_unknown' else ('unknown: usage_not_reported' if a is None or b is None else 'undefined_zero_denominator' if b==0 else f'{a/b:.4f}')
            lines.append(f'| {model} | {k} | {ratio} |')
    lines+=['','## Oráculos y fallos conservados','']
    for model in MODELS:
        for phase in ('baseline','after'):
            row=rows[(phase,model)]
            if not row: continue
            events=[json.loads(s) for s in (DATA/phase/model/'events.jsonl').read_text().splitlines()]
            assistants=[e['message'] for e in events if e.get('type')=='message_end' and e.get('message',{}).get('role')=='assistant']
            apis=sorted({m.get('api','unknown') for m in assistants})
            last=''.join(c.get('text','') for c in assistants[-1].get('content',[]) if c.get('type')=='text') if assistants else ''
            try:
                final_obj=json.loads(last)
                json_only=isinstance(final_obj,dict) and all(k in final_obj for k in ('located_file','diagnostic_before_test','fixed_caller'))
            except ValueError: json_only=False
            silent_help=[]
            for e in events:
                if e.get('type')!='tool_execution_end' or e.get('toolName')!='bench_command': continue
                r=e.get('result',{})
                if 'diagnostics' in r.get('details',{}).get('command',''):
                    body=''.join(c.get('text','') for c in r.get('content',[]) if c.get('type')=='text')
                    if 'Available Commands:' in body: silent_help.append(e['toolCallId'])
            lines += [f'### {model} / {phase}', '',
                      f"- Ruta solicitada: `{row['model_requested']}`. Observada: `{', '.join(row['model_observed']) or 'unknown'}`.",
                      f"- Modelo inmutable: **unknown**; alias fijado, revisión no publicada. Pi: `{row['harness']}`. APIs observadas: `{apis}`.",
                      f"- Salida final exclusivamente JSON con las tres claves: `{json_only}`. Esto se informa separado del score de las tres tareas, no se oculta el incumplimiento de formato.",
                      f"- Falsos éxitos funcionales: `{len(silent_help)}` respuestas de diagnostics con ayuda en vez de diagnósticos y exit 0; no aparecen como isError en la métrica nativa. IDs: `{silent_help}`.",
                      f"- Oráculos: `{json.dumps(row['oracle'],ensure_ascii=False)}`.",
                      f"- Exit: `{row['exit_code']}`; HTTP observados: `{row['http_statuses']}`.",
                      f"- Errores proveedor: `{json.dumps(row['provider_errors'])}`; harness: `{json.dumps(row['harness_errors'])}`.",
                      f"- Archivos read: `{row['explicit_files_read']}`; backend transitivo: **unknown**.",
                      f"- Datos: `results/harness-measurement/{phase}/{model}/` (invocation.json, events.jsonl, guard.jsonl, metrics.json, final-src/).",'']
    lines+=['## Alcance y entrega','',
            '- Contrato: `measurement-contract.md`.',
            '- Receta y oráculos: `benchmarks/harness-measurement/{run.py,report.py,prompt.md,guard.ts,seed/greeter.go}`.',
            '- Guía: `docs/benchmarks/HARNESS_MEASUREMENT.md`.',
            '- Binario baseline congelado: `results/harness-measurement/bin/mi-lsp-baseline` (no publicar).',
            '- After: el padre provee binario integrado + SHA y lanza ambos modelos en FINAL_VERIFY. No ejecutar código fragmentado de lanes.',
            '- No se afirma causalidad ni mejora con N=1; la versión opaca del modelo, proveedor free, caché y carga son confusores. No acredita CC como host ni mi-mcp.',
            '- Sin cambios de producto, suites, commit/push, release o instalación global. Estado de lane: `lane_implementation_done`; ambos intentos baseline terminales fueron inspeccionados. After sigue `pending_final_verify` con dueño padre.', '']
    (ROOT/'results/measurement-baseline.md').write_text('\n'.join(lines))
    print('results/measurement-baseline.md')

if __name__=='__main__': main()
