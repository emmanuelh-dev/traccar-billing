DROP INDEX idx_accounts_seller;

ALTER TABLE accounts DROP COLUMN seller_id;

DROP INDEX idx_sellers_tenant;

DROP TABLE sellers;
