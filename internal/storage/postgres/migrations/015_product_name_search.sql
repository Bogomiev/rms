-- Was 012_product_name_search.sql; renumbered after 012-014 (stoppers). Safe to rerun on
-- databases that applied it under the old name.
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE INDEX IF NOT EXISTS idx_product_name_trgm ON products USING GIN (name public.gin_trgm_ops);
-- The old name is forgotten, so schema_migrations keeps one record of this migration.
DELETE FROM schema_migrations WHERE version='012_product_name_search.sql';
