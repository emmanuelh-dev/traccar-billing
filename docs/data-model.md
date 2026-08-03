# Modelo de datos

Migraciones en `migrations/`, un juego por motor (`sqlite/` y `mysql/`).
**Toda migración nueva se escribe en los dos**, con su `.down.sql`. Van
embebidas en el binario (`migrations/embed.go`) y se aplican al arrancar.

## Esquema actual (migraciones 1–3)

```
tenants
  id, name, base_url (UNIQUE)
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

## Esquema objetivo

### Migración 4 — precio por dispositivo y modo de ciclo

```sql
ALTER TABLE subscriptions ADD COLUMN unit_price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN flat_fee_cents   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN min_devices      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN billing_mode     TEXT    NOT NULL DEFAULT 'rolling';
ALTER TABLE subscriptions ADD COLUMN anchor_day       INTEGER NOT NULL DEFAULT 1;
ALTER TABLE subscriptions ADD COLUMN due_day          INTEGER NOT NULL DEFAULT 5;
ALTER TABLE subscriptions ADD COLUMN grace_days       INTEGER NOT NULL DEFAULT 0;
```

`unit_price_cents = 0` conserva el comportamiento actual (`amount_cents` fijo),
así que las cuentas existentes no cambian de monto.

También hace falta la unicidad que el código ya asume:

```sql
CREATE UNIQUE INDEX idx_subscriptions_account_unique ON subscriptions(account_id);
```

### Migración 5 — remisiones

```sql
CREATE TABLE statements (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id        INTEGER NOT NULL REFERENCES tenants(id),
    account_id       INTEGER NOT NULL REFERENCES accounts(id),
    serie            TEXT    NOT NULL DEFAULT 'A',
    folio            INTEGER NOT NULL,
    period_start     DATETIME NOT NULL,
    period_end       DATETIME NOT NULL,
    device_count     INTEGER NOT NULL,          -- congelado al emitir
    unit_price_cents INTEGER NOT NULL,
    flat_fee_cents   INTEGER NOT NULL DEFAULT 0,
    subtotal_cents   INTEGER NOT NULL,
    tax_cents        INTEGER NOT NULL DEFAULT 0, -- 0 hasta que haya CFDI
    total_cents      INTEGER NOT NULL,
    paid_cents       INTEGER NOT NULL DEFAULT 0,
    currency         TEXT    NOT NULL,
    issued_at        DATETIME NOT NULL,
    due_at           DATETIME NOT NULL,
    status           TEXT    NOT NULL DEFAULT 'open',  -- open|partial|paid|overdue|void
    void_reason      TEXT    NOT NULL DEFAULT '',
    voided_at        DATETIME,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_statements_period ON statements(account_id, period_start);
CREATE UNIQUE INDEX idx_statements_folio  ON statements(tenant_id, serie, folio);
CREATE INDEX idx_statements_status ON statements(tenant_id, status, due_at);
```

`UNIQUE(account_id, period_start)` es lo que hace idempotente al generador —
es la pieza crítica de toda la fase.

### Migración 6 — pagos ligados a remisión

```sql
ALTER TABLE payments ADD COLUMN statement_id INTEGER REFERENCES statements(id);
ALTER TABLE payments ADD COLUMN device_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payments ADD COLUMN method       TEXT NOT NULL DEFAULT 'cash';
ALTER TABLE payments ADD COLUMN reference    TEXT NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN voided_at    DATETIME;
ALTER TABLE payments ADD COLUMN void_reason  TEXT NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN created_by   TEXT NOT NULL DEFAULT '';
```

`statement_id` es nullable: un anticipo o un pago suelto no tiene remisión.
`device_count` guarda cuántos dispositivos se cobraron en ese pago concreto —
es lo que captura el modal de cobro.

Los pagos no se borran nunca. Corregir = `voided_at` + registrar el correcto.

### Migración 7 — preparación fiscal (sin timbrar)

```sql
ALTER TABLE accounts ADD COLUMN tax_id      TEXT NOT NULL DEFAULT '';  -- RFC
ALTER TABLE accounts ADD COLUMN legal_name  TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN tax_regime  TEXT NOT NULL DEFAULT '';  -- régimen SAT
ALTER TABLE accounts ADD COLUMN cfdi_use    TEXT NOT NULL DEFAULT '';  -- uso CFDI
ALTER TABLE accounts ADD COLUMN tax_zip     TEXT NOT NULL DEFAULT '';

ALTER TABLE statements ADD COLUMN fiscal_uuid TEXT NOT NULL DEFAULT '';
ALTER TABLE statements ADD COLUMN stamped_at  DATETIME;
```

Se capturan y se guardan desde ya; el timbrado con un PAC llega después
([roadmap.md](roadmap.md), Fase 5). Cuando llegue, no hay que migrar datos
históricos ni pedirle el RFC a nadie otra vez.

### Migración 8 — bajas y auditoría

```sql
ALTER TABLE accounts ADD COLUMN archived_at DATETIME;  -- desapareció de Traccar

CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id  INTEGER NOT NULL,
    actor      TEXT    NOT NULL,     -- operador o "scheduler"
    action     TEXT    NOT NULL,     -- payment.record, statement.void, account.suspend...
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

`billing.Repository` hoy solo expone operaciones sueltas. Cobrar toca tres
tablas (`payments`, `statements`, `subscriptions`) y **tiene que ser atómico**.
Hace falta agregar a la interfaz:

```go
WithTx(ctx context.Context, fn func(Repository) error) error
```

SQLite y MySQL lo soportan igual. Sin esto, un fallo a medio cobro deja la
cuenta al corriente sin pago registrado — que es lo que pasa hoy.
