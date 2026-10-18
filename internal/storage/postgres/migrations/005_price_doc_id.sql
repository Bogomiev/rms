-- Keep existing snapshots until the next GetPrices refresh.
ALTER TABLE prices ADD COLUMN doc_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
DO $$
DECLARE pk_name TEXT;
BEGIN
 SELECT conname INTO pk_name FROM pg_constraint WHERE conrelid='prices'::regclass AND contype='p';
 EXECUTE format('ALTER TABLE prices DROP CONSTRAINT %I',pk_name);
END $$;
-- Staging tables survive snapshot swaps and must have the same schema.
DROP TABLE IF EXISTS prices_staging;
ALTER TABLE prices ADD PRIMARY KEY (period, price_type, product_id, doc_id);
