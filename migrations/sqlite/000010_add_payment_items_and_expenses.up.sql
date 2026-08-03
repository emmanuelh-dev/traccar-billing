CREATE TABLE payments_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id),
    subscription_id INTEGER REFERENCES subscriptions(id),
    concept_id INTEGER REFERENCES concepts(id),
    amount_cents INTEGER NOT NULL DEFAULT 0,
    unit_price_cents INTEGER NOT NULL DEFAULT 0,
    device_count INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'MXN',
    method TEXT NOT NULL DEFAULT '',
    reference TEXT NOT NULL DEFAULT '',
    paid_at DATETIME NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    voided_at DATETIME,
    void_reason TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Nullable on purpose: payments written before migration 000005 have no
    -- updated_at, and NOT NULL here would abort the copy below.
    updated_at DATETIME
);

INSERT INTO payments_new (id, account_id, subscription_id, concept_id, amount_cents, unit_price_cents, device_count, currency, method, reference, paid_at, note, voided_at, void_reason, created_at, updated_at)
SELECT p.id, s.account_id, p.subscription_id, p.concept_id, p.amount_cents, p.unit_price_cents, p.device_count, p.currency, p.method, p.reference, p.paid_at, p.note, p.voided_at, p.void_reason, p.created_at, p.updated_at
FROM payments p
JOIN subscriptions s ON s.id = p.subscription_id;

DROP TABLE payments;

ALTER TABLE payments_new RENAME TO payments;

CREATE INDEX idx_payments_subscription ON payments(subscription_id);
CREATE INDEX idx_payments_paid_at ON payments(paid_at);
CREATE INDEX idx_payments_concept ON payments(concept_id);
CREATE INDEX idx_payments_account ON payments(account_id);

CREATE TABLE payment_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    payment_id INTEGER NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    concept_id INTEGER REFERENCES concepts(id),
    description TEXT NOT NULL DEFAULT '',
    quantity INTEGER NOT NULL DEFAULT 1,
    unit_price_cents INTEGER NOT NULL DEFAULT 0,
    amount_cents INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_payment_items_payment ON payment_items(payment_id);

CREATE TABLE expenses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id),
    seller_id INTEGER REFERENCES sellers(id),
    category TEXT NOT NULL DEFAULT '',
    amount_cents INTEGER NOT NULL,
    currency TEXT NOT NULL DEFAULT 'MXN',
    spent_at DATETIME NOT NULL,
    method TEXT NOT NULL DEFAULT '',
    reference TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_expenses_tenant_date ON expenses(tenant_id, spent_at);
