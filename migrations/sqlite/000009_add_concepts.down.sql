DROP INDEX idx_payments_concept;

ALTER TABLE payments DROP COLUMN concept_id;

DROP INDEX idx_concepts_tenant;

DROP TABLE concepts;
