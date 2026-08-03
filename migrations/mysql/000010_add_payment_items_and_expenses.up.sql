ALTER TABLE payments ADD COLUMN account_id BIGINT NULL;

UPDATE payments p
JOIN subscriptions s ON s.id = p.subscription_id
SET p.account_id = s.account_id;

ALTER TABLE payments MODIFY COLUMN account_id BIGINT NOT NULL;
ALTER TABLE payments MODIFY COLUMN subscription_id BIGINT NULL;
ALTER TABLE payments ADD CONSTRAINT fk_payments_account FOREIGN KEY (account_id) REFERENCES accounts(id);

CREATE INDEX idx_payments_account ON payments(account_id);

CREATE TABLE payment_items (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    payment_id BIGINT NOT NULL,
    concept_id BIGINT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    quantity INT NOT NULL DEFAULT 1,
    unit_price_cents BIGINT NOT NULL DEFAULT 0,
    amount_cents BIGINT NOT NULL DEFAULT 0,
    position INT NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_payment_items_payment FOREIGN KEY (payment_id) REFERENCES payments(id) ON DELETE CASCADE,
    CONSTRAINT fk_payment_items_concept FOREIGN KEY (concept_id) REFERENCES concepts(id)
) ENGINE=InnoDB;

CREATE INDEX idx_payment_items_payment ON payment_items(payment_id);

CREATE TABLE expenses (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    seller_id BIGINT NULL,
    category VARCHAR(191) NOT NULL DEFAULT '',
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'MXN',
    spent_at DATETIME NOT NULL,
    method VARCHAR(191) NOT NULL DEFAULT '',
    reference VARCHAR(191) NOT NULL DEFAULT '',
    note VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_expenses_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_expenses_seller FOREIGN KEY (seller_id) REFERENCES sellers(id)
) ENGINE=InnoDB;

CREATE INDEX idx_expenses_tenant_date ON expenses(tenant_id, spent_at);
