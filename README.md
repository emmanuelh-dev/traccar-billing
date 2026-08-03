# traccar-billing

**Español | [English](README.en.md)**

[![CI](https://github.com/yourusername/traccar-billing/actions/workflows/ci.yml/badge.svg)](https://github.com/yourusername/traccar-billing/actions/workflows/ci.yml)
[![Go Reference](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**traccar-billing** es un servicio de cobro/facturación multitenant para
servidores [Traccar](https://www.traccar.org), la plataforma open-source de
rastreo GPS. Se conecta a uno o varios servidores Traccar existentes,
sincroniza sus cuentas/usuarios, y lleva el control de cobro (suscripciones
y pagos) de cada una. Corre como un solo binario en Go, sin
dependencias externas más que la base de datos.

Es **multitenant**: puedes conectar más de un servidor Traccar (por ejemplo,
si administras Traccar para varios clientes), cada uno con su propia sesión
y sus propias cuentas.

## Qué hace al arrancar

1. Lee la configuración desde variables de entorno (falla de inmediato si
   falta algo crítico).
2. Aplica las migraciones de base de datos pendientes.
3. Levanta un scheduler en segundo plano que, cada `SYNC_INTERVAL`, sincroniza
   usuarios/dispositivos de cada tenant conectado y revisa qué suscripciones
   están vencidas.
4. Levanta el servidor web (login + dashboard + API JSON).
5. Se apaga limpio al recibir `Ctrl+C` o una señal de sistema (`SIGTERM`).

## Cómo conectar un servidor Traccar

No se configura por variable de entorno (guardar la contraseña de Traccar en
texto plano en un `.env` no es seguro). En vez de eso:

1. Abre `http://localhost:8083/login` en el navegador.
2. Mete la URL de tu servidor Traccar (con o sin `/api` al final, da igual),
   tu correo y tu contraseña.
3. El servicio inicia sesión contra tu Traccar, y **solo guarda la cookie de
   sesión resultante, nunca la contraseña**.
4. Si esa sesión llega a vencer, la próxima vez que entres al dashboard te
   pedirá la contraseña de nuevo. No hay forma de saltarse esto por
   variable de entorno a propósito.

## Cómo correrlo (bare metal, sin Docker)

Necesitas [Go 1.26+](https://go.dev/dl/) instalado.

```bash
# 1. Copia el archivo de ejemplo de variables de entorno
cp .env.example .env

# 2. Genera un SESSION_SECRET real y pégalo en .env
openssl rand -hex 32

# 3. Corre el servicio (usa SQLite por defecto, no necesita nada más)
make run
```

Esto deja el servicio escuchando en `http://localhost:8083`. Abre
`http://localhost:8083/login` en el navegador.

Si no tienes `make`, el equivalente directo es:

```bash
export $(grep -v '^#' .env | xargs)
go run ./cmd/traccar-billing
```

Para compilar un binario (queda en `bin/traccar-billing`):

```bash
make build
./bin/traccar-billing
```

## Cómo correrlo con Docker

```bash
docker compose up --build
```

Esto levanta el servicio junto con una base de datos MySQL de prueba (ver
`docker-compose.yml`). Cambia `SESSION_SECRET` en ese archivo antes de
usarlo en serio.

## Variables de entorno

Ver `.env.example` para la lista completa con comentarios. Resumen:

| Variable | Requerida | Descripción |
|---|---|---|
| `HTTP_PORT` | no (default `8083`) | Puerto del servidor web. |
| `DB_DRIVER` | no (default `sqlite`) | `mysql` o `sqlite`. |
| `DATABASE_URL` | sí | DSN de la base de datos. |
| `SYNC_INTERVAL` | no (default `15m`) | Cada cuánto sincroniza y revisa vencidos. |
| `SESSION_SECRET` | sí | Firma la cookie de sesión del navegador. `openssl rand -hex 32`. |

## API

Para uso humano:
- `GET /login`, `POST /login`, `POST /logout`
- `GET /dashboard` — cuentas y estado de pago del tenant autenticado

Para integraciones (requieren la cookie de sesión del navegador):
- `GET /accounts` — lista cuentas del tenant con su estado de pago
- `GET /accounts/{id}` — detalle + historial de pagos
- `POST /accounts/{id}/pay` — registra un pago manual. Si la cuenta no
  tiene suscripción todavía, hay que mandar `amount_cents` y `period_days`
  en el body para crear la primera; si ya existe, esos campos son opcionales.
- `GET /health` — healthcheck, sin autenticación

## Pendiente / roadmap

- **Webhooks entrantes y salientes.** Por ahora todo el flujo de pagos es
  manual (`POST /accounts/{id}/pay`) y toda la sincronización es por
  polling (`SYNC_INTERVAL`). Falta: webhooks salientes para notificar a
  sistemas externos cuando una cuenta se marca vencida o recibe un pago, y
  webhooks entrantes para que una pasarela de pagos (Stripe, Conekta, etc.)
  registre pagos automáticamente en vez de hacerlo a mano.
- **Sistema de tickets.** Idea a futuro, todavía sin definir (¿tickets por
  cuenta? ¿prioridad, estado, asignación?). Se define antes de construirse.
