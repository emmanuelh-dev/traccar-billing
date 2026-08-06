# traccar-billing

**[English](README.md) | Español**

[![CI](https://github.com/emmanuelh-dev/traccar-billing/actions/workflows/ci.yml/badge.svg)](https://github.com/emmanuelh-dev/traccar-billing/actions/workflows/ci.yml)
[![Go Reference](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**El sistema de cobranza que le falta a [Traccar](https://www.traccar.org).**

Traccar es excelente rastreando, pero no sabe nada de dinero: no sabe quién te
pagó, quién te debe, ni a quién hay que cortarle el servicio. Eso normalmente
se termina llevando en una hoja de cálculo, y la hoja de cálculo no apaga
cuentas.

traccar-billing se conecta a tu servidor Traccar, se trae sus usuarios, y les
pone encima todo lo que sí es un negocio: suscripciones, pagos, conceptos de
cobro, vendedores con comisión, gastos y agenda de instalaciones. Cuando una
cuenta se vence, **le quita el acceso en Traccar**; cuando paga, se lo
devuelve.

Es un solo binario de Go. No necesita nada más que una base de datos.

---

## En una frase

> Un panel donde ves quién debe, cobras, y el corte de servicio se aplica solo.

## Para quién es

Para quien **revende rastreo GPS**: montaste un Traccar, tienes clientes con
uno o con doscientos equipos, cobras mensualidad, cobras instalaciones, pagas
comisiones a vendedores y llevas gastos. Si administras Traccar para varios
negocios distintos, es **multitenant**: cada servidor Traccar es un tenant
aparte, con su propia sesión, sus cuentas y sus números.

---

## Qué resuelve

### Cobranza que sí corta el servicio

Cada cuenta de Traccar puede tener una suscripción: precio, periodo y fecha de
vencimiento. El scheduler revisa periódicamente qué venció y, pasados los días
de gracia, **deshabilita al usuario en Traccar**. Al registrar el pago se
reactiva y la fecha avanza. Nadie tiene que acordarse de hacerlo a mano.

Hay dos modos de facturación:

- **Rolling** — el periodo corre desde la fecha de pago (30 días y otros 30).
- **Calendario** — todos vencen el mismo día del mes (por ejemplo, día 5).

### Cobro por dispositivo

El precio se define **por equipo**, más una cuota fija opcional y un mínimo de
dispositivos. Como traccar-billing ya sabe cuántos equipos tiene cada cuenta
(los lee de Traccar), el cobro se calcula solo y se actualiza cuando el cliente
da de alta o de baja un GPS.

### Cobros de varias líneas

Un cobro no siempre es "la mensualidad". Puedes cobrar en un solo movimiento
varias líneas — mensualidad + instalación + un cable — cada una con concepto,
cantidad y precio unitario.

La distinción que importa: un concepto marcado como **no recurrente** es un
**cargo único**. Cobra, pero **no mueve la fecha de vencimiento ni reactiva el
servicio**. Así puedes cobrarle una instalación a alguien que no tiene
mensualidad, sin regalarle un mes.

### Vendedores

Cada cuenta se asigna a un vendedor, con su porcentaje de comisión. El tablero
se agrupa por vendedor y te dice cuánto trajo cada uno.

### Gastos

Lo que sale del cajón: pago al instalador, compra de equipo, comisiones,
gasolina. Con eso el periodo ya no muestra solo lo cobrado, sino el **neto**.
La categoría sugiere las opciones de siempre y las que tú ya hayas usado, sin
obligarte a una taxonomía cerrada.

### Agenda de instalaciones

Las visitas se agendan **antes de que el cliente exista**: cliente, fecha,
horario, contacto, unidad, dirección, cuántas altas y cuánto se cobra. Cada
visita se cierra o se cancela con un motivo, y la que ya pasó de fecha y sigue
abierta se marca **Atrasada**. Botón de **WhatsApp** por contacto, con el
mensaje de confirmación ya escrito.

### El día a día

- Dashboard en tabla o en tarjetas, con totales y ordenamiento configurable
  (por vendedor, por monto, por vencimiento) que se recuerda entre sesiones.
- Historial de pagos con edición, cancelación y borrado.
- Catálogo de conceptos por tenant.
- Español e inglés.
- Funciona en el teléfono: menú lateral tipo hamburguesa.
- Las cuentas espejo de Traccar (los usuarios temporales que crea al compartir
  un dispositivo) se ocultan solas: no son clientes.

---

## Cómo conectar un servidor Traccar

**No se configura por variable de entorno**, a propósito: guardar la contraseña
de Traccar en texto plano en un `.env` no es seguro.

1. Abre `http://localhost:8083/login`.
2. Mete la URL de tu Traccar (con o sin `/api` al final, da igual), tu correo y
   tu contraseña.
3. El servicio inicia sesión contra tu Traccar y **solo guarda la cookie de
   sesión resultante, nunca la contraseña**. El navegador recuerda la URL del
   servidor para la próxima vez.
4. Si esa sesión vence, el dashboard te pide la contraseña otra vez. No hay
   forma de saltárselo, y es intencional.

---

## Cómo correrlo

Necesitas [Go 1.26+](https://go.dev/dl/).

```bash
cp .env.example .env      # 1. variables de entorno
openssl rand -hex 32      # 2. genera SESSION_SECRET y pégalo en .env
make run                  # 3. arranca (SQLite por defecto)
```

Queda en `http://localhost:8083`. Sin `make`:

```bash
export $(grep -v '^#' .env | xargs)
go run ./cmd/traccar-billing
```

Para compilar (`bin/traccar-billing`):

```bash
make build
```

### Con Docker

```bash
docker compose up --build
```

Levanta el servicio con un MySQL de prueba (ver `docker-compose.yml`). Cambia
`SESSION_SECRET` antes de usarlo en serio.

### Variables de entorno

| Variable | Requerida | Descripción |
|---|---|---|
| `DATABASE_URL` | **sí** | DSN de la base de datos. |
| `SESSION_SECRET` | **sí** | Firma la cookie de sesión. `openssl rand -hex 32`. |
| `DB_DRIVER` | no (`sqlite`) | `sqlite` o `mysql`. |
| `HTTP_PORT` | no (`8083`) | Puerto del servidor web. |
| `SYNC_INTERVAL` | no (`15m`) | Cada cuánto sincroniza y revisa vencidos. |
| `TIMEZONE` | no (`UTC`) | Zona horaria para fechas y vencimientos. |

Lista completa y comentada en `.env.example`.

Las credenciales de proveedores de SIM no son variables de entorno. Cada
usuario configura su propia cuenta de 1GLOBAL desde **Ajustes → Proveedor de
SIM**; la API key se valida y se almacena cifrada.

---

## Qué hace al arrancar

1. Lee la configuración; si falta algo crítico falla de inmediato en vez de
   arrancar a medias.
2. Aplica las migraciones pendientes (SQLite y MySQL van en paralelo, cada
   cambio de esquema existe para ambos).
3. Levanta el scheduler: cada `SYNC_INTERVAL` sincroniza usuarios y
   dispositivos de cada tenant y aplica los vencimientos.
4. Levanta el servidor web.
5. Se apaga limpio con `Ctrl+C` o `SIGTERM`.

---

## API

Páginas (sesión de navegador): `/dashboard`, `/payments`, `/expenses`,
`/appointments`, `/sellers`, `/concepts`, `/settings`.

JSON, con la misma cookie de sesión:

- `GET /accounts` — cuentas del tenant con su estado de pago
- `GET /accounts/{id}` — detalle e historial
- `POST /accounts/{id}/pay` — registra un pago. Si la cuenta aún no tiene
  suscripción, manda `amount_cents` y `period_days` para crear la primera;
  si ya existe, son opcionales.
- `POST /accounts/{id}/subscription` — configura precio y periodo
- `GET /health` — healthcheck, sin autenticación

---

## Stack

Go 1.26, [chi](https://github.com/go-chi/chi), `html/template` con los
templates embebidos en el binario, SQLite o MySQL. **Sin framework de front y
sin build step**: HTML server-side y CSS propio.

## Documentación

El modelo de datos, las reglas de cobro y la arquitectura están en
[`docs/`](docs/README.md). Si vas a tocar código, empieza por
[`docs/decisions.md`](docs/decisions.md) — hay cosas que parecen bugs y son
decisiones deliberadas.

## Roadmap

Plan por fases en [`docs/roadmap.md`](docs/roadmap.md). Lo más grande que
falta:

- **Remisiones automáticas** al cierre de cada periodo, calculadas según los
  dispositivos que tenga la cuenta en ese momento.
- **Portal para clientes**: que cada cliente vea su estado de cuenta y su
  historial. Requiere autenticación separada de la de operadores.
- **Cobranza automática**: recordatorios por correo/WhatsApp antes del
  vencimiento y aviso antes de suspender.
- **Facturación CFDI (México)**: los datos fiscales se capturan desde antes;
  el timbrado con un PAC viene después.
- **Webhooks**: salientes para avisar a sistemas externos cuando una cuenta
  vence o paga; entrantes para que una pasarela (Stripe, Conekta) registre los
  pagos sin intervención.
- **Tickets de soporte**: idea a futuro, todavía sin definir.

## Licencia

[MIT](LICENSE).
