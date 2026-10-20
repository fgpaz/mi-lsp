---
wiki_contract: WIKI-CONTRACT-v1
id: RS-MILSP-RUNTIME
kind: outcome
status: active
title: Uso local portable con daemon opcional y diagnóstico explícito
---

## Resultado observable

La CLI funciona con selección de workspace confiable y sin exigir daemon. Los runtimes especializados y la consola loopback permiten operar y diagnosticar el sistema sin convertir estado local en configuración portable.

## Beneficiario

Desarrolladores, agentes y operadores locales del daemon.

## Criterios de éxito

- Init y el registro conservan la topología single/container y las raíces de canon.
- Los workers se instalan explícitamente; no se descargan durante una consulta.
- Fallback, carga de runtime, estado y errores siguen visibles y accionables.
- Probe no crea DB, WAL/SHM, registry ni telemetría.
- La consola muestra workspaces, runtimes, accesos y logs del daemon actual; el warm no crea otra instancia.
- La telemetría contiene agregados sanitizados, no patrones, contenido, tokens ni secretos.

## Fuera de alcance del RS

Setup remoto, dashboard público, nueva puerta MCP, migraciones automáticas de estado o almacenamiento de secretos en la wiki.
