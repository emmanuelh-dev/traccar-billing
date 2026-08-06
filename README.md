# traccar-billing

**English | [Español](README.es.md)**

[![CI](https://github.com/emmanuelh-dev/traccar-billing/actions/workflows/ci.yml/badge.svg)](https://github.com/emmanuelh-dev/traccar-billing/actions/workflows/ci.yml)
[![Go Reference](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**The billing system [Traccar](https://www.traccar.org) is missing.**

Traccar is great at tracking, but it knows nothing about money: not who paid
you, not who owes you, not whose service should be cut off. That usually ends
up in a spreadsheet, and a spreadsheet cannot disable an account.

traccar-billing connects to your Traccar server, pulls in its users, and layers
on everything that makes it a business: subscriptions, payments, billing
concepts, sellers with commission, expenses and an installation schedule. When
an account goes overdue it **disables the user in Traccar**; when they pay, it
puts the access back.

It is a single Go binary. It needs nothing but a database.

---

## In one line

> A panel where you see who owes you, take the payment, and the service cutoff
> applies itself.

## Who it is for

Anyone **reselling GPS tracking**: you run a Traccar, you have customers with
one device or two hundred, you bill monthly, you bill installations, you pay
seller commissions and you track expenses. If you run Traccar for several
separate businesses it is **multi-tenant**: each Traccar server is its own
tenant, with its own session, accounts and numbers.

---

## What it solves

### Billing that actually cuts off service

Every Traccar account can carry a subscription: price, period and due date. The
scheduler checks what is overdue and, once the grace days are up, **disables
the user in Traccar**. Recording the payment reactivates them and rolls the
date forward. Nobody has to remember to do it by hand.

Two billing modes:

- **Rolling** — the period runs from the payment date (30 days, then another 30).
- **Calendar** — everyone is due on the same day of the month (say, the 5th).

### Per-device pricing

Price is set **per device**, plus an optional flat fee and a device minimum.
Since traccar-billing already knows how many devices each account has (it reads
them from Traccar), the charge is computed for you and follows the customer as
they add or remove units.

### Multi-line charges

A charge is not always "the monthly fee". One payment can carry several lines —
monthly + installation + a cable — each with its own concept, quantity and unit
price.

The distinction that matters: a concept marked **non-recurring** is a
**one-off charge**. It collects money but **does not move the due date and does
not restore service**. That is how you bill an installation to someone with no
monthly plan without handing them a free month.

### Sellers

Each account is assigned to a seller with a commission rate. The dashboard
groups by seller and tells you how much each one brought in.

### Expenses

What leaves the register: paying the installer, buying hardware, commissions,
fuel. With that, a period shows not just what came in but the **net**. The
category field suggests the usual reasons plus whatever you have already typed,
without forcing a taxonomy on you.

### Installation schedule

Visits are booked **before the customer exists**: client, date, time window,
contact, unit, address, how many activations and what it costs. Each visit is
closed or canceled with a reason, and one still open past its date is flagged
**Late**. A **WhatsApp** button per contact opens the chat with the
confirmation message already written.

### Day to day

- Dashboard as a table or as cards, with totals and configurable sorting (by
  seller, amount, due date) remembered between sessions.
- Payment history with edit, void and delete.
- Per-tenant concept catalog.
- Spanish and English.
- Works on a phone: off-canvas hamburger menu.
- Traccar's mirror accounts (the temporary users it creates when sharing a
  device) hide themselves — they are not customers.

---

## Connecting a Traccar server

**Not configured through environment variables**, on purpose: keeping a Traccar
password in plain text in a `.env` is not safe.

1. Open `http://localhost:8083/login`.
2. Enter your Traccar URL (with or without a trailing `/api`, either works),
   your email and your password.
3. The service logs into your Traccar and **stores only the resulting session
   cookie, never the password**. The browser remembers the server URL for next
   time.
4. If that session expires, the dashboard asks for the password again. There is
   no way around it, and that is deliberate.

---

## Running it

You need [Go 1.26+](https://go.dev/dl/).

```bash
cp .env.example .env      # 1. environment variables
openssl rand -hex 32      # 2. generate SESSION_SECRET, paste it into .env
make run                  # 3. start (SQLite by default)
```

It listens on `http://localhost:8083`. Without `make`:

```bash
export $(grep -v '^#' .env | xargs)
go run ./cmd/traccar-billing
```

To build (`bin/traccar-billing`):

```bash
make build
```

### With Docker

```bash
docker compose up --build
```

Brings up the service with a throwaway MySQL (see `docker-compose.yml`). Change
`SESSION_SECRET` before using it for real.

### Environment variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | **yes** | Database DSN. |
| `SESSION_SECRET` | **yes** | Signs the browser session cookie. `openssl rand -hex 32`. |
| `DB_DRIVER` | no (`sqlite`) | `sqlite` or `mysql`. |
| `HTTP_PORT` | no (`8083`) | Web server port. |
| `SYNC_INTERVAL` | no (`15m`) | How often it syncs and checks for overdue accounts. |
| `TIMEZONE` | no (`UTC`) | Time zone for dates and due dates. |

Full commented list in `.env.example`.

SIM provider credentials are not environment variables. Each user configures
their own 1GLOBAL account under **Settings → SIM provider**; the API key is
validated and stored encrypted.

---

## What happens on startup

1. Reads configuration; if something critical is missing it fails immediately
   rather than starting half-configured.
2. Applies pending migrations (SQLite and MySQL are kept in lockstep — every
   schema change exists for both).
3. Starts the scheduler: every `SYNC_INTERVAL` it syncs each tenant's users and
   devices and applies overdue rules.
4. Starts the web server.
5. Shuts down cleanly on `Ctrl+C` or `SIGTERM`.

---

## API

Pages (browser session): `/dashboard`, `/payments`, `/expenses`,
`/appointments`, `/sellers`, `/concepts`, `/settings`.

JSON, using the same session cookie:

- `GET /accounts` — tenant accounts with billing status
- `GET /accounts/{id}` — detail and payment history
- `POST /accounts/{id}/pay` — record a payment. If the account has no
  subscription yet, send `amount_cents` and `period_days` to create the first
  one; if it exists, both are optional.
- `POST /accounts/{id}/subscription` — configure price and period
- `GET /health` — healthcheck, unauthenticated

---

## Stack

Go 1.26, [chi](https://github.com/go-chi/chi), `html/template` with templates
embedded in the binary, SQLite or MySQL. **No frontend framework and no build
step**: server-rendered HTML and hand-written CSS.

## Documentation

Data model, billing rules and architecture live in [`docs/`](docs/README.md).
If you are going to touch the code, start with
[`docs/decisions.md`](docs/decisions.md) — some things look like bugs and are
deliberate.

## Roadmap

Phased plan in [`docs/roadmap.md`](docs/roadmap.md). The big missing pieces:

- **Automatic invoices** at the end of each period, computed from the devices
  the account has at that moment.
- **Customer portal**: let each customer see their statement and history.
  Needs customer authentication, separate from operator auth.
- **Automated dunning**: email/WhatsApp reminders before the due date and a
  warning before suspension.
- **CFDI invoicing (Mexico)**: tax data gets captured up front; stamping
  through a PAC comes later.
- **Webhooks**: outbound to notify external systems when an account goes
  overdue or pays; inbound so a payment gateway (Stripe, Conekta) can record
  payments without anyone typing them.
- **Support tickets**: a future idea, still undefined.

## License

[MIT](LICENSE).
