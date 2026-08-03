ALTER TABLE accounts DROP FOREIGN KEY fk_accounts_seller;
ALTER TABLE accounts DROP COLUMN seller_id;

DROP INDEX idx_sellers_tenant ON sellers;

DROP TABLE sellers;
