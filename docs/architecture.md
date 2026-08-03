# Arquitectura

## Paquetes

```
cmd/traccar-billing/main.go   arranque: config → migraciones → scheduler → servidor HTTP
internal/config/              variables de entorno, falla rápido si falta algo
internal/billing/             el dominio. Structs y reglas puras, sin I/O.
internal/storage/             implementación de billing.Repository (SQLite y MySQL)
internal/traccar/             cliente HTTP de la API de Traccar
internal/scheduler/           trabajo en segundo plano: sincronizar y cortar morosos
internal/api/                 handlers HTTP, plantillas, sesión de navegador, i18n
migrations/                   SQL embebido, un juego por motor (sqlite/, mysql/)
```

La regla de dependencias: **`internal/billing` no importa a nadie más.** Define
las interfaces (`Repository`, `TraccarClient`) y los demás paquetes las
implementan. Si necesitas una consulta nueva, se declara en
`billing/repository.go` y se implementa en `storage/queries.go`.

`billing/rules.go` es lógica pura y determinista (recibe `now` como parámetro,
nunca llama a `time.Now()`). Eso es a propósito: es lo único que se puede
probar sin base de datos ni red. Mantenlo así.

## Flujo de una petición del dashboard

1. `api.Router()` (chi) recibe la petición.
2. `requireTenant` (`middleware.go`) lee la cookie de sesión firmada,
   saca el `tenant_id`, carga el tenant y lo mete en el contexto.
3. El handler consulta el repositorio y arma una *view struct*
   (ej. `dashboardView`) — las plantillas nunca reciben modelos del dominio
   crudos, siempre una struct de vista con los valores ya formateados.
4. `render()` (`templates.go`) ejecuta la plantilla embebida.

Las plantillas y el CSS están embebidos en el binario con `go:embed`, así que
el ejecutable no necesita archivos externos.

## Autenticación (dos niveles distintos, no los confundas)

- **Sesión de Traccar (del tenant):** cuando conectas un servidor Traccar,
  el servicio hace login contra su API y guarda **solo la cookie resultante**
  en `tenants.session_cookie`. Nunca la contraseña. Esa cookie es la que usa
  el scheduler para leer usuarios/dispositivos y para deshabilitar morosos.
  Si vence, hay que volver a meter la contraseña en `/login`.
- **Sesión del navegador (del operador):** una cookie propia firmada con
  HMAC usando `SESSION_SECRET` (`api/session.go`). Solo dice "este navegador
  está viendo el tenant N".

Hoy **no hay usuarios operadores locales**: quien tenga las credenciales de
Traccar entra. Ver la Fase 4 del roadmap.

## Scheduler

`scheduler.Run` corre `runOnce` al arrancar y luego cada `SYNC_INTERVAL`
(default 15 min). Por cada tenant, con un timeout de 30 s:

1. **`syncTenant`** — trae los usuarios de Traccar, y por cada uno sus
   dispositivos (`FetchDevicesForUser`, porque los dispositivos de Traccar no
   traen dueño), y hace upsert de la cuenta con su `device_count`.
2. **`checkOverdue`** — busca suscripciones vencidas, las marca `overdue` y
   deshabilita al usuario en Traccar. Reintenta el corte **en cada tick**, no
   solo en la transición, para que una caída de Traccar se auto-repare sola.

Protección importante: nunca deshabilita al usuario administrador con cuya
sesión está trabajando (`tenants.admin_traccar_user_id`) — si no, el servicio
se cortaría a sí mismo el acceso.

Si Traccar responde 401, se borra la cookie guardada y se sigue: se considera
estado normal, no un fallo del ciclo.

## Dónde falla hoy este diseño

- **El corte depende de una cookie que vence.** Sin sesión válida, el
  scheduler solo escribe un `Warn` en el log y deja de suspender morosos.
  Nadie se entera. Falta alerta visible en el dashboard.
- **No hay transacciones.** `UpsertSubscription` y `RecordPayment` son dos
  llamadas separadas (`api/handlers_accounts.go`); si la segunda falla, la
  cuenta queda al corriente sin pago registrado.
- **El sync es solo aditivo.** `syncTenant` nunca marca una cuenta como
  eliminada cuando el usuario desaparece de Traccar.
