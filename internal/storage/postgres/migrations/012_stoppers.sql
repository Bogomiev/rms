CREATE TABLE stoppers (
 id SMALLSERIAL PRIMARY KEY,
 name TEXT NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 24),
 color TEXT NOT NULL CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_stoppers_name ON stoppers (lower(name));
-- Stoppers from the promotion regulation.
INSERT INTO stoppers (name,color) VALUES ('Выгодно','#F2B705'),('ХИТ','#D7261E'),('Новинка','#1E9E4A'),('Распродажа','#E8731A');
-- A promotional position is a product of a 1C promotion document; doc_id is NULL for
-- products added by hand. A row exists only after an admin changed the stopper or period.
CREATE TABLE stopper_positions (
 id BIGSERIAL PRIMARY KEY,
 product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
 doc_id UUID,
 stopper_id SMALLINT REFERENCES stoppers(id) ON DELETE SET NULL,
 period_from DATE NOT NULL,
 period_to DATE CHECK (period_to >= period_from),
 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_stopper_positions_promo ON stopper_positions (doc_id,product_id) WHERE doc_id IS NOT NULL;
CREATE UNIQUE INDEX idx_stopper_positions_manual ON stopper_positions (product_id) WHERE doc_id IS NULL;
CREATE INDEX idx_stopper_positions_stopper ON stopper_positions (stopper_id);
CREATE TRIGGER stoppers_update_at BEFORE UPDATE ON stoppers FOR EACH ROW EXECUTE FUNCTION touch_task_update_at();
CREATE TRIGGER stopper_positions_update_at BEFORE UPDATE ON stopper_positions FOR EACH ROW EXECUTE FUNCTION touch_task_update_at();
