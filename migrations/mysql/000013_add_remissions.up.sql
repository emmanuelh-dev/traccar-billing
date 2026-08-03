CREATE TABLE remissions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    subscription_id BIGINT NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    device_count INT NOT NULL DEFAULT 0,
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(191) NOT NULL,
    status VARCHAR(191) NOT NULL DEFAULT 'pending',
    note VARCHAR(500) NOT NULL DEFAULT '',
    payment_id BIGINT NULL,
    issued_at DATETIME NOT NULL,
    paid_at DATETIME NULL,
    canceled_at DATETIME NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_remissions_period (subscription_id, period_start),
    CONSTRAINT fk_remissions_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_remissions_account FOREIGN KEY (account_id) REFERENCES accounts(id),
    CONSTRAINT fk_remissions_subscription FOREIGN KEY (subscription_id) REFERENCES subscriptions(id),
    CONSTRAINT fk_remissions_payment FOREIGN KEY (payment_id) REFERENCES payments(id)
) ENGINE=InnoDB;

CREATE INDEX idx_remissions_tenant_status ON remissions(tenant_id, status);
