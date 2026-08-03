CREATE TABLE appointments (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    -- Nullable on purpose: a visit is usually booked before the customer
    -- exists in Traccar, so it may have no account and no seller yet.
    account_id BIGINT NULL,
    seller_id BIGINT NULL,
    client_name VARCHAR(191) NOT NULL DEFAULT '',
    phone VARCHAR(191) NOT NULL DEFAULT '',
    unit VARCHAR(191) NOT NULL DEFAULT '',
    address VARCHAR(500) NOT NULL DEFAULT '',
    device_count INT NOT NULL DEFAULT 1,
    amount_cents BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(8) NOT NULL DEFAULT 'MXN',
    scheduled_on DATE NOT NULL,
    time_window VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'scheduled',
    note VARCHAR(500) NOT NULL DEFAULT '',
    outcome VARCHAR(500) NOT NULL DEFAULT '',
    closed_at DATETIME NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_appointments_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_appointments_account FOREIGN KEY (account_id) REFERENCES accounts(id),
    CONSTRAINT fk_appointments_seller FOREIGN KEY (seller_id) REFERENCES sellers(id)
) ENGINE=InnoDB;

CREATE INDEX idx_appointments_tenant_date ON appointments(tenant_id, scheduled_on);
CREATE INDEX idx_appointments_status ON appointments(tenant_id, status);
