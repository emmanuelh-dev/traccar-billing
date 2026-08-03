ALTER TABLE payments DROP FOREIGN KEY fk_payments_concept;
ALTER TABLE payments DROP COLUMN concept_id;

DROP INDEX idx_concepts_tenant ON concepts;

DROP TABLE concepts;
