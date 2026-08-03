CREATE TABLE appointments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id),
    -- Nullable on purpose: a visit is usually booked before the customer
    -- exists in Traccar, so it may have no account and no seller yet.
    account_id INTEGER REFERENCES accounts(id),
    seller_id INTEGER REFERENCES sellers(id),
    client_name TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    device_count INTEGER NOT NULL DEFAULT 1,
    amount_cents INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'MXN',
    scheduled_on DATE NOT NULL,
    time_window TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'scheduled',
    note TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT '',
    closed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_appointments_tenant_date ON appointments(tenant_id, scheduled_on);
CREATE INDEX idx_appointments_status ON appointments(tenant_id, status);
