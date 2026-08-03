CREATE TABLE concepts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    amount_cents INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'MXN',
    recurring INTEGER NOT NULL DEFAULT 0,
    active INTEGER NOT NULL DEFAULT 1,
    note TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_concepts_tenant ON concepts(tenant_id, active);

INSERT INTO concepts (tenant_id, slug, name, recurring, amount_cents, currency) SELECT id, 'instalacion', 'Instalación', 0, 0, 'MXN' FROM tenants;
INSERT INTO concepts (tenant_id, slug, name, recurring, amount_cents, currency) SELECT id, 'mensualidad', 'Mensualidad', 1, 0, 'MXN' FROM tenants;
INSERT INTO concepts (tenant_id, slug, name, recurring, amount_cents, currency) SELECT id, 'desinstalacion', 'Desinstalación', 0, 0, 'MXN' FROM tenants;

ALTER TABLE payments ADD COLUMN concept_id INTEGER REFERENCES concepts(id);

CREATE INDEX idx_payments_concept ON payments(concept_id);
