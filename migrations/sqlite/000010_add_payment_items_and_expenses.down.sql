DROP INDEX IF EXISTS idx_expenses_tenant_date;
DROP TABLE IF EXISTS expenses;

DROP INDEX IF EXISTS idx_payment_items_payment;
DROP TABLE IF EXISTS payment_items;

DELETE FROM payments WHERE subscription_id IS NULL;

CREATE TABLE payments_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id),
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
    updated_at DATETIME
);

INSERT INTO payments_old (id, subscription_id, concept_id, amount_cents, unit_price_cents, device_count, currency, method, reference, paid_at, note, voided_at, void_reason, created_at, updated_at)
SELECT id, subscription_id, concept_id, amount_cents, unit_price_cents, device_count, currency, method, reference, paid_at, note, voided_at, void_reason, created_at, updated_at
FROM payments;

DROP TABLE payments;

ALTER TABLE payments_old RENAME TO payments;

CREATE INDEX idx_payments_subscription ON payments(subscription_id);
CREATE INDEX idx_payments_paid_at ON payments(paid_at);
CREATE INDEX idx_payments_concept ON payments(concept_id);
