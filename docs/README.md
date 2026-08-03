# Documentación de traccar-billing

Esta carpeta existe para no tener que leer todo el código cada vez que
retomas el proyecto. Empieza por aquí.

| Documento | Qué contiene |
|---|---|
| [architecture.md](architecture.md) | Cómo está armado: paquetes, flujo de una petición, qué hace el scheduler. |
| [data-model.md](data-model.md) | Esquema actual, esquema objetivo y las migraciones que faltan. |
| [billing-rules.md](billing-rules.md) | Las reglas de negocio: ciclos, remisiones, morosidad, suspensión. **Léelo antes de tocar `internal/billing`.** |
| [decisions.md](decisions.md) | Decisiones tomadas a propósito que parecen bugs. Léelo antes de "arreglar" algo. |
| [roadmap.md](roadmap.md) | El plan por fases, con el estado de cada punto. |
| [operations.md](operations.md) | Correr, configurar, migrar, respaldar, actualizar. |

## Resumen en 30 segundos

traccar-billing es un servicio en Go (un solo binario) que se conecta a uno o
varios servidores [Traccar](https://www.traccar.org), sincroniza sus usuarios
como *cuentas*, y lleva el cobro de cada una: cuánto pagan, cuándo vence,
qué han pagado. Cuando una cuenta se vence, deshabilita al usuario en Traccar;
cuando paga, lo vuelve a habilitar.

Es multitenant: cada servidor Traccar conectado es un *tenant*, con su propia
sesión y sus propias cuentas.

**Destino del proyecto:** producto para otros administradores de Traccar, no
solo herramienta interna. Eso sube el listón en UI, respaldos, roles y
facilidad de instalación — ver [roadmap.md](roadmap.md).
