---
wiki_contract: WIKI-CONTRACT-v1
id: FL-SEM-01
kind: flow
status: active
title: Recuperación semántica documental con proveedor opcional
links:
  fulfills: [RS-MILSP-NAVIGATION]
---

## Objetivo

Recuperar secciones documentales por significado con embeddings pluggables, indexación incremental y rerank local opcional. Se materializa el flujo ya declarado en el inventario legacy y en TECH-SEMANTIC-RECALL; no se agrega capacidad.

## Alcance

Wikis gobernadas y de conocimiento. La configuración del proveedor es explícita; no se embebe código ni se usa un proveedor oculto.

## Actores

Usuario o agente, CLI/core, embed, wikichunk, almacén repo-local y comando local opcional de rerank.

## Pre/postcondiciones

El proveedor requiere base_url y model en project.toml, salvo enabled=false. Sin configuración se devuelve hint. Un chunk persistido se reutiliza cuando hash, modelo y dimensión coinciden. Un fallo conserva los lotes exitosos; cancelación no certifica completitud.

## Secuencia

El diagrama separa indexación y consulta: una llamada a recall no implica reindexar. Los participantes y condiciones proceden de los actores, precondiciones y pasos de este flujo; embed y wikichunk forman parte del core.

```mermaid
sequenceDiagram
    participant U as Usuario o agente
    participant C as CLI y core (embed / wikichunk)
    participant S as Almacén repo-local
    participant P as Proveedor de embeddings configurado
    participant R as Comando local de rerank
    alt Indexación documental
        U->>C: Indexar documentos
        C->>S: Publica corpus documental
        opt Embeddings activados
            C->>C: Detecta cambios de hash, metadata, modelo o dimensión
            C->>P: Codifica chunks afectados
            P-->>C: Vectores o fallo del proveedor
            C->>S: Conserva lotes confirmados y progreso exitoso
            opt Fase completada
                C->>S: Pruning de filas inválidas o stale
            end
        end
    else Consulta recall
        U->>C: Consulta semántica
        alt Sin configuración activa
            C-->>U: Hint de configuración
        else Proveedor configurado
            C->>P: Codifica la consulta
            P-->>C: Vector o fallo explícito
            alt Proveedor disponible
                C->>S: Lee chunks y vectores persistidos
                S-->>C: Candidatos documentales
                C->>C: Similitud coseno y ranking determinista
                opt Rerank local configurado
                    C->>R: Ejecuta comando sin shell
                    R-->>C: Resultado o fallo
                    C->>C: Aplica resultado válido; ante fallo conserva orden semántico
                end
                C-->>U: Candidatos, score, why, presupuesto y warnings
            else Fallo del proveedor
                C-->>U: Degradación y hints según CT-NAV-RECALL
            end
        end
    end
    Note over U,C: Los candidatos no sustituyen al canon
```

1. La indexación publica el corpus documental y procesa embeddings activados.
2. Se detectan cambios por metadata-prefix, contenido, modelo y dimensión.
3. Los lotes confirmados se guardan incrementalmente; el pruning solo ocurre al completar la fase.
4. Recall codifica la consulta, aplica similitud coseno y ranking determinista.
5. Si está configurado, el comando externo de rerank se ejecuta sin shell; un fallo conserva el orden semántico.
6. La respuesta conserva campos, score, why, presupuesto y warnings. Los candidatos se contrastan con el canon antes de usarse como autoridad.

## Restricciones ADR

Sin CGO ni extensión sqlite-vec; no cliente privado de rerank en el core. Recall no requiere gobernanza válida ni daemon. No persiste consultas, snippets, stdout/stderr, tokens ni secretos. Se conservan la degradación y los hints documentados en CT-NAV-RECALL y TECH-SEMANTIC-RECALL, sin modificar el runtime.
