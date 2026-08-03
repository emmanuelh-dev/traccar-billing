# Modelo de datos

Migraciones en `migrations/`, un juego por motor (`sqlite/` y `mysql/`).
**Toda migración nueva se escribe en los dos**, con su `.down.sql`. Van
embebidas en el binario (`migrations/embed.go`) y se aplican al arrancar.

## Esquema actual (migraciones 1–3)

```
tenants                                  un usuario de Traccar, no un servidor
  id, name, base_url
  traccar_user_id                        UNIQUE(base_url, traccar_user_id)
  owner_email                            para mostrar de quién es la sesión
  session_cookie, session_expires_at     cookie de Traccar, nunca la contraseña
  admin_traccar_user_id                  usuario protegido: nunca se corta
  created_at, updated_at

accounts
  id, tenant_id → tenants
  traccar_user_id                        UNIQUE(tenant_id, traccar_user_id)
  name, email
  device_count                           lo llena el scheduler en cada sync
  created_at, updated_at

subscriptions
  id, account_id → accounts
  status                                 active | overdue | suspended | canceled
  amount_cents, currency
  period_days
  last_paid_at, next_due_at
  created_at, updated_at

payments
  id, subscription_id → subscriptions
  amount_cents, currency
  paid_at, note
  created_at
```

Índices: `accounts(tenant_id)`, `subscriptions(account_id)`,
`subscriptions(next_due_at)`, `payments(subscription_id)`.

### Limitaciones del esquema actual

- Un pago no sabe **qué periodo** cubre. No se puede reimprimir un recibo ni
  aclarar una disputa.
- Un pago no se puede anular ni corregir. No hay `voided_at` ni autor.
- `device_count` se sincroniza pero no participa en el cobro.
- No hay dónde poner datos fiscales.
- `subscriptions` no tiene restricción de una por cuenta, aunque el código
  asume que hay una (`GetSubscriptionByAccountID`).

## Migraciones 4 y 5 (aplicadas)

### 000004 — precio por dispositivo

```sql
ALTER TABLE subscriptions ADD COLUMN unit_price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN flat_fee_cents   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN min_devices      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN grace_days       INTEGER NOT NULL DEFAULT 0;
```

`unit_price_cents = 0` conserva el comportamiento anterior (`amount_cents`
como precio fijo), así que ninguna cuenta existente cambió de monto al migrar.
El cálculo vive en `billing.ChargeCents`.

### 000005 — detalle del pago

```sql
ALTER TABLE payments ADD COLUMN device_count     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payments ADD COLUMN unit_price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payments ADD COLUMN method           TEXT NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN reference        TEXT NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN voided_at        DATETIME;
ALTER TABLE payments ADD COLUMN void_reason      TEXT NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN updated_at       DATETIME;
```

`device_count` y `unit_price_cents` guardan lo que realmente se cobró en ese
pago, que no tiene por qué coincidir con la configuración vigente de la
suscripción. `voided_at` permite corregir sin borrar: los pagos no se
eliminan nunca.

### 000006 — bajas y ciclo por calendario

```sql
ALTER TABLE accounts ADD COLUMN archived_at DATETIME;
ALTER TABLE subscriptions ADD COLUMN billing_mode TEXT NOT NULL DEFAULT 'rolling';
ALTER TABLE subscriptions ADD COLUMN anchor_day INTEGER NOT NULL DEFAULT 1;
ALTER TABLE subscriptions ADD COLUMN due_day INTEGER NOT NULL DEFAULT 5;
```

`ListAccountsByTenant` excluye las archivadas; `UpsertAccount` limpia
`archived_at`, así que un usuario que reaparece en Traccar revive con su
historial. El scheduler solo archiva **después de un fetch completo y
exitoso**: una lista parcial archivaría todo lo que no alcanzó a ver.

### 000007 — vendedores

```sql
CREATE TABLE sellers (
    id, tenant_id, name, email, phone,
    commission_bp INTEGER NOT NULL DEFAULT 0,
    active, note, created_at, updated_at
);
ALTER TABLE accounts ADD COLUMN seller_id INTEGER REFERENCES sellers(id);
```

`commission_bp` son puntos base (1000 = 10%), enteros como todo el dinero.
`seller_id` es nullable: una cuenta sin vendedor es válida.

## Esquema objetivo

### Migración 8 — remisiones

```sql
CREATE TABLE statements (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id        INTEGER NOT NULL REFERENCES tenants(id),
    account_id       INTEGER NOT NULL REFERENCES accounts(id),
    serie            TEXT    NOT NULL DEFAULT 'A',
    folio            INTEGER NOT NULL,
    period_start     DATETIME NOT NULL,
    period_end       DATETIME NOT NULL,
    device_count     INTEGER NOT NULL,
    unit_price_cents INTEGER NOT NULL,
    flat_fee_cents   INTEGER NOT NULL DEFAULT 0,
    subtotal_cents   INTEGER NOT NULL,
    tax_cents        INTEGER NOT NULL DEFAULT 0,
    total_cents      INTEGER NOT NULL,
    paid_cents       INTEGER NOT NULL DEFAULT 0,
    currency         TEXT    NOT NULL,
    issued_at        DATETIME NOT NULL,
    due_at           DATETIME NOT NULL,
    status           TEXT    NOT NULL DEFAULT 'open',
    void_reason      TEXT    NOT NULL DEFAULT '',
    voided_at        DATETIME,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_statements_period ON statements(account_id, period_start);
CREATE UNIQUE INDEX idx_statements_folio  ON statements(tenant_id, serie, folio);
CREATE INDEX idx_statements_status ON statements(tenant_id, status, due_at);

ALTER TABLE payments ADD COLUMN statement_id INTEGER REFERENCES statements(id);
```

`UNIQUE(account_id, period_start)` es lo que hace idempotente al generador —
es la pieza crítica de toda la fase. `statement_id` queda nullable: un
anticipo o un pago suelto no tiene remisión.

Falta también el modo de ciclo por calendario, que va junto con esto:

```sql
ALTER TABLE subscriptions ADD COLUMN billing_mode TEXT NOT NULL DEFAULT 'rolling';
ALTER TABLE subscriptions ADD COLUMN anchor_day   INTEGER NOT NULL DEFAULT 1;
ALTER TABLE subscriptions ADD COLUMN due_day      INTEGER NOT NULL DEFAULT 5;
```

Y la unicidad que el código ya asume:

```sql
CREATE UNIQUE INDEX idx_subscriptions_account_unique ON subscriptions(account_id);
```

### Migración 9 — preparación fiscal (sin timbrar)

```sql
ALTER TABLE accounts ADD COLUMN tax_id      TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN legal_name  TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN tax_regime  TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cfdi_use    TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN tax_zip     TEXT NOT NULL DEFAULT '';

ALTER TABLE statements ADD COLUMN fiscal_uuid TEXT NOT NULL DEFAULT '';
ALTER TABLE statements ADD COLUMN stamped_at  DATETIME;
```

Se capturan y se guardan desde ya; el timbrado con un PAC llega después
([roadmap.md](roadmap.md), Fase 5). Cuando llegue, no hay que migrar datos
históricos ni pedirle el RFC a nadie otra vez.

### Migración 10 — auditoría

```sql
CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id  INTEGER NOT NULL,
    actor      TEXT    NOT NULL,
    action     TEXT    NOT NULL,
    entity     TEXT    NOT NULL,
    entity_id  INTEGER NOT NULL,
    detail     TEXT    NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_tenant ON audit_log(tenant_id, created_at);
```

Las cuentas archivadas dejan de generar remisiones y no aparecen en el
dashboard por default, pero conservan su historial.

## Transacciones

`billing.Repository` expone:

```go
WithTx(ctx context.Context, fn func(Repository) error) error
```

El repositorio que recibe `fn` corre todo sobre la misma transacción; anidar
`WithTx` reutiliza la que ya está abierta en vez de abrir otra. Registrar un
pago toca `payments` y `subscriptions` y **pasa completo por ahí**: sin eso,
un fallo a medio cobro dejaba la cuenta al corriente sin pago registrado.

Ojo con SQLite: el pool está limitado a una conexión
(`db.SetMaxOpenConns(1)`), así que dentro de un `WithTx` nunca hay que usar
el repositorio de afuera — se traba. Usa el que recibe la función.
