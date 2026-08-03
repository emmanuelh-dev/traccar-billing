# Operación

## Configuración

Todo por variables de entorno (`internal/config/config.go`). Falla al
arrancar si falta algo crítico — a propósito, para no descubrirlo en
producción.

| Variable | Default | Notas |
|---|---|---|
| `HTTP_PORT` | `8083` | |
| `DB_DRIVER` | `sqlite` | `sqlite` o `mysql` |
| `DATABASE_URL` | — | **requerida.** sqlite: ruta del archivo. mysql: DSN con `?parseTime=true` |
| `SYNC_INTERVAL` | `15m` | cada cuánto sincroniza y revisa vencidos |
| `SESSION_SECRET` | — | **requerida**, mínimo 32 caracteres. `openssl rand -hex 32` |

Las credenciales de Traccar **no** se configuran aquí: cada tenant se agrega
desde `/login` y solo se guarda la cookie resultante. Ver
[decisions.md](decisions.md#2-nunca-se-guarda-la-contraseña-de-traccar).

## Correr

```bash
cp .env.example .env
openssl rand -hex 32     # pégalo en SESSION_SECRET
make run                 # http://localhost:8083
```

```bash
make build   # bin/traccar-billing, estático (CGO_ENABLED=0)
make test
make lint    # go vet + golangci-lint
```

Con Docker: `docker compose up --build` (levanta MySQL 8.4).
**Cambia el `SESSION_SECRET` del compose antes de exponerlo** — trae un valor
de ejemplo.

## Migraciones

Están en `migrations/{sqlite,mysql}/`, embebidas en el binario y aplicadas
automáticamente al arrancar (`internal/storage/migrate.go`).

Al agregar una:

1. Escríbela en **los dos** motores, con su `.down.sql`.
2. Numeración consecutiva: `000004_lo_que_sea.up.sql`.
3. SQLite no soporta todos los `ALTER TABLE` de MySQL; si hace falta, usa el
   patrón de tabla nueva + copia + rename.
4. Pruébala contra una copia de la base real, no solo contra una vacía.

## Respaldos

**Todavía no hay nada automatizado** — es un pendiente de la Fase 4 y es lo
primero que va a preguntar quien te confíe su cobranza.

Mientras tanto, con SQLite:

```bash
sqlite3 traccar-billing.db ".backup respaldo-$(date +%F).db"
```

No copies el archivo con `cp` mientras el servicio corre. Con MySQL, el
`mysqldump` de siempre.

## Diagnóstico

- **No aparecen las cuentas después de conectar un Traccar.** Hoy el sync
  ocurre en el siguiente tick del scheduler, hasta `SYNC_INTERVAL` después.
  Es un pendiente conocido (Fase 1). Baja `SYNC_INTERVAL` o reinicia el
  servicio, que sincroniza al arrancar.
- **Dejaron de cortarse los morosos.** Casi siempre es la sesión de Traccar
  vencida. Busca `tenant session expired` o `no valid session` en el log y
  vuelve a entrar en `/login`. Hoy no hay aviso visible — pendiente de Fase 0.
- **Una cuenta no se deshabilita.** Revisa que no sea el usuario
  administrador del tenant: está protegido a propósito y solo deja un `Warn`.
- `/health` hoy solo responde `{"status":"ok"}`; **no** verifica la base de
  datos ni la sesión de Traccar. No lo uses como señal de que todo está bien.

## Logs

`slog` en texto a stdout. Los mensajes útiles llevan prefijo del paquete
(`scheduler:`, `api:`) y campos estructurados (`tenant_id`, `account_id`).
