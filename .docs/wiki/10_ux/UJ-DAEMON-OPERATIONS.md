---
wiki_contract: WIKI-CONTRACT-v1
id: UJ-DAEMON-OPERATIONS
kind: journey
status: active
title: Diagnosticar y calentar un workspace desde la consola local
links:
  experiences: [RS-MILSP-RUNTIME]
  stages: [FL-DAE-01]
---

## Objetivo

Hacer visible el daemon actual, seleccionar un workspace y diagnosticar su runtime sin otra instancia ni una GUI remota.

## Recorrido

1. El operador abre admin_url con workspace y panel opcionales.
2. La consola enfoca el workspace solicitado y muestra overview, runtimes o logs.
3. Los filtros acotan backend, cliente y ventana de eventos.
4. Warm workspace solicita el calentamiento del workspace enfocado; Refresh vuelve a leer estado.
5. Open logs muestra la cola y Copy CLI ofrece la orden del contexto seleccionado.

## Estados y límites

La respuesta real determina listo, vacío o error; un acceso fallido no significa actividad cero. Las acciones y el mensaje deben conservar el contexto del workspace. No se exponen tokens del daemon ni contenido sensible. Se describe la UI existente, no una pantalla nueva.
