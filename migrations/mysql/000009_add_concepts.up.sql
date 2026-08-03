CREATE TABLE concepts (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(191) NOT NULL,
    slug VARCHAR(191) NOT NULL,
    amount_cents BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(8) NOT NULL DEFAULT 'MXN',
    recurring TINYINT(1) NOT NULL DEFAULT 0,
    active TINYINT(1) NOT NULL DEFAULT 1,
    note VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_concepts_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    UNIQUE KEY uq_concepts_tenant_slug (tenant_id, slug)
) ENGINE=InnoDB;

CREATE INDEX idx_concepts_tenant ON concepts(tenant_id, active);

INSERT INTO concepts (tenant_id, slug, name, recurring, amount_cents, currency) SELECT id, 'instalacion', 'Instalación', 0, 0, 'MXN' FROM tenants;
INSERT INTO concepts (tenant_id, slug, name, recurring, amount_cents, currency) SELECT id, 'mensualidad', 'Mensualidad', 1, 0, 'MXN' FROM tenants;
INSERT INTO concepts (tenant_id, slug, name, recurring, amount_cents, currency) SELECT id, 'desinstalacion', 'Desinstalación', 0, 0, 'MXN' FROM tenants;

ALTER TABLE payments ADD COLUMN concept_id BIGINT NULL;
ALTER TABLE payments ADD CONSTRAINT fk_payments_concept FOREIGN KEY (concept_id) REFERENCES concepts(id);
