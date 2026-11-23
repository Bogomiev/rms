-- When RMS first received a 1C price change of the day: 1C sends no time, so the hourly sync keeps the time of
-- a row it already has (same store, product, promotional flag and price) and stamps new or changed rows.
-- A change that comes after a tag was printed the same day puts the product to print again.
ALTER TABLE today_price_changes ADD COLUMN arrived_at TIMESTAMPTZ NOT NULL DEFAULT now();
