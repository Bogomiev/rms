ALTER TABLE products ADD COLUMN images JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(images) = 'array');
ALTER TABLE stores ADD COLUMN price_type UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
CREATE TABLE prices (
 period TIMESTAMP NOT NULL,
 price_type UUID NOT NULL,
 product_id UUID NOT NULL,
 price NUMERIC NOT NULL,
 PRIMARY KEY (period, price_type, product_id)
);
CREATE INDEX ON prices (price_type, product_id, period);
CREATE TABLE stocks (
 product_id UUID NOT NULL,
 store_id UUID NOT NULL,
 stock NUMERIC NOT NULL,
 PRIMARY KEY (store_id, product_id)
);
