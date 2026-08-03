# traccar-billing

**[Español](README.md) | English**

[![CI](https://github.com/yourusername/traccar-billing/actions/workflows/ci.yml/badge.svg)](https://github.com/yourusername/traccar-billing/actions/workflows/ci.yml)
[![Go Reference](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**traccar-billing** is a multi-tenant billing service for
[Traccar](https://www.traccar.org) servers, the open-source GPS tracking
platform. It connects to one or more existing Traccar servers, syncs their
accounts/users, and tracks billing (subscriptions and payments) for each
one. It runs as a single Go binary, with no external dependencies beyond
the database.

It is **multi-tenant**: you can connect more than one Traccar server (for
example, if you manage Traccar for several clients), each with its own
session and its own accounts.

## What it does on startup

1. Reads configuration from environment variables (fails immediately if
   anything critical is missing).
2. Applies pending database migrations.
3. Starts a background scheduler that, every `SYNC_INTERVAL`, syncs
   users/devices for each connected tenant and checks which subscriptions
   are overdue.
4. Starts the web server (login + dashboard + JSON API).
5. Shuts down cleanly on `Ctrl+C` or a system signal (`SIGTERM`).

## How to connect a Traccar server

It is not configured via environment variable (storing a Traccar password
in plaintext in a `.env` file is not secure). Instead:

1. Open `http://localhost:8083/login` in your browser.
2. Enter your Traccar server URL (with or without `/api` at the end, it
   does not matter), your email, and your password.
3. The service signs in against your Traccar server, and **only stores the
   resulting session cookie, never the password**.
4. If that session expires, the next time you visit the dashboard it will
   ask for the password again. There is no way to bypass this via
   environment variable, by design.

## How to run it (bare metal, no Docker)

You need [Go 1.26+](https://go.dev/dl/) installed.

```bash
# 1. Copy the example environment file
cp .env.example .env

# 2. Generate a real SESSION_SECRET and paste it into .env
openssl rand -hex 32

# 3. Run the service (uses SQLite by default, no extra setup needed)
make run
```

This leaves the service listening on `http://localhost:8083`. Open
`http://localhost:8083/login` in your browser.

If you don't have `make`, the direct equivalent is:

```bash
export $(grep -v '^#' .env | xargs)
go run ./cmd/traccar-billing
```

To build a binary (ends up at `bin/traccar-billing`):

```bash
make build
./bin/traccar-billing
```

## How to run it with Docker

```bash
docker compose up --build
```

This starts the service alongside a test MySQL database (see
`docker-compose.yml`). Change `SESSION_SECRET` in that file before using it
for anything real.

## Environment variables

See `.env.example` for the full list with comments. Summary:

| Variable | Required | Description |
|---|---|---|
| `HTTP_PORT` | no (default `8083`) | Web server port. |
| `DB_DRIVER` | no (default `sqlite`) | `mysql` or `sqlite`. |
| `DATABASE_URL` | yes | Database DSN. |
| `SYNC_INTERVAL` | no (default `15m`) | How often it syncs and checks for overdue accounts. |
| `SESSION_SECRET` | yes | Signs the browser session cookie. `openssl rand -hex 32`. |

## API

For human use:
- `GET /login`, `POST /login`, `POST /logout`
- `GET /dashboard` — accounts and payment status for the authenticated tenant

For integrations (require the browser session cookie):
- `GET /accounts` — lists the tenant's accounts with their payment status
- `GET /accounts/{id}` — detail + payment history
- `POST /accounts/{id}/pay` — records a manual payment. If the account
  doesn't have a subscription yet, you must send `amount_cents` and
  `period_days` in the body to create the first one; if one already
  exists, those fields are optional.
- `GET /health` — healthcheck, no authentication required

## Documentation

The full plan, data model and billing rules live in
[`docs/`](docs/README.md) (in Spanish). If you're going to touch the code,
start with [`docs/decisions.md`](docs/decisions.md) — some things that look
like bugs are deliberate.

## Pending / roadmap

The detailed phased plan is in [`docs/roadmap.md`](docs/roadmap.md). The
biggest missing pieces:

- **Automatic statements.** At the end of each period (say, generated on the
  1st and due on the 5th), each account's statement should be produced
  automatically, priced from however many devices it has at that moment.
- **Per-device pricing.** Configure a unit price per account, and when
  recording a payment, ask how many devices are being charged (prefilled
  with the account's current count) and compute the total.
- **Customer portal.** Let each customer log in to see their balance,
  statements and payment history. Needs customer authentication, separate
  from operator authentication, and really pays off alongside online
  payments.
- **Automated collections.** Email/WhatsApp reminders before the due date
  and a warning before service is suspended.
- **CFDI invoicing (Mexico).** Tax data (RFC, tax regime, CFDI use) will be
  captured ahead of time; stamping through a PAC comes later.
- **Incoming and outgoing webhooks.** For now the whole payment flow is
  manual (`POST /accounts/{id}/pay`) and all syncing is by polling
  (`SYNC_INTERVAL`). Missing: outgoing webhooks to notify external systems
  when an account is marked overdue or receives a payment, and incoming
  webhooks so a payment gateway (Stripe, Conekta, etc.) can register
  payments automatically instead of doing it by hand.
- **Ticketing system.** Future idea, not yet scoped (per-account tickets?
  priority, status, assignment?). Will be scoped before it's built.
