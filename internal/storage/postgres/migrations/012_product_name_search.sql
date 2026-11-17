CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE INDEX idx_product_name_trgm ON products USING GIN (name public.gin_trgm_ops);
