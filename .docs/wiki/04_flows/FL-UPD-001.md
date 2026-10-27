---
wiki_contract: WIKI-CONTRACT-v1
id: FL-UPD-001
kind: flow
status: active
title: Autoactualización del ejecutable con un único dueño runtime
links:
  fulfills:
  - RS-MILSP-RUNTIME
---

## Objetivo

El runtime mi-lsp mantiene su ejecutable actualizado para todos los consumidores, sin actualizadores paralelos en plugins ni cortes de requests.

## Alcance

CLI portable Windows x64/arm64, Linux x64/arm64 y macOS x64/arm64, incluidas instalaciones de mis-entornos en user-bin y ~/.local/bin. No descarga workers durante consultas ni administra versiones de otras herramientas.

## Actores

CLI, scheduler del daemon, GitHub Releases oficial y hooks consumidores del estado. El hook notifica; no instala.

## Pre/postcondiciones

Home e instalación accesibles sin admin. Un pin explícito u opt-out impide aplicar. La versión publicada debe superar la instalada y respetar el piso. Tras éxito, installed refleja readback real; el daemon viejo puede seguir atendiendo hasta reposo comprobado.

## Secuencia

1. Un comando normal programa un hijo desconectado si no hay daemon compatible dueño de esa instalación. No espera a la red.
2. El daemon programa por su cuenta. El lock OS y caché global del home limitan metadata automática a una vez cada 24 horas, incluidos fallos de red.
3. Otra instalación con el mismo home reutiliza metadata validada pero conserva estado propio por ejecutable canónico.
4. latest y el asset exacto del RID se obtienen sólo del repositorio oficial. Se valida SHA-256 contra el archivo de checksums; nunca se usa un digest alternativo.
5. Se extrae únicamente la CLI a staging adyacente. Windows renombra a .old; Unix promociona con rename atómico.
6. Se ejecuta version con marcador antirrecursión. Sólo una promoción y readback exitosos publican updated con previous anterior.
7. El siguiente arranque normal limpia un backup propio e inalterado. No toca backups ajenos ni fuerza eliminación Windows en uso.
8. El daemon sólo se recicla tras cerrar admisión y comprobar ausencia de requests, respuestas pendientes, HTTP, actividad de fondo y sesiones/contextos vivos. Lo desconocido difiere el reciclado.

## Restricciones ADR

Un único dueño runtime, sin privilegios adicionales. MCP stdio adopta la versión nueva en la sesión siguiente; un daemon compartido ajeno no se detiene. Detalle y límites: [[TECH-RUNTIME-UPDATE]], [[RF-UPD-001]], [[TP-UPD-001]].
