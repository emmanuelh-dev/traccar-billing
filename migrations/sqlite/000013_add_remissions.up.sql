CREATE TABLE remissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id),
    account_id INTEGER NOT NULL REFERENCES accounts(id),
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id),
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    device_count INTEGER NOT NULL DEFAULT 0,
    amount_cents INTEGER NOT NULL,
    currency TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    note TEXT NOT NULL DEFAULT '',
    payment_id INTEGER REFERENCES payments(id),
    issued_at DATETIME NOT NULL,
    paid_at DATETIME,
    canceled_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uq_remissions_period ON remissions(subscription_id, period_start);
CREATE INDEX idx_remissions_tenant_status ON remissions(tenant_id, status);
