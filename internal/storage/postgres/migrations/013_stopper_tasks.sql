ALTER TABLE task_types DROP CONSTRAINT task_types_service_check;
ALTER TABLE task_types ADD CONSTRAINT task_types_service_check CHECK (service IN ('cash_collection','price_print','without_service','stoppers'));
INSERT INTO task_types (name,short_name,service,color,color_dark,sort_order)
VALUES ('Стопперы','Стопперы','stoppers','#FBEADB','#3A2817',0) ON CONFLICT (name) DO NOTHING;
-- Tasks created by hand from the store page; schedule reconciliation keeps them.
ALTER TABLE tasks ADD COLUMN manual BOOLEAN NOT NULL DEFAULT false;
-- Store results for positions of a «Стопперы» task: ok/bad for stoppers to check,
-- removed for stoppers whose period ended. doc_id is NULL for manual positions.
CREATE TABLE stopper_task_checks (
 id BIGSERIAL PRIMARY KEY,
 task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 product_id UUID NOT NULL,
 doc_id UUID,
 result TEXT NOT NULL CHECK (result IN ('ok','bad','removed')),
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 checked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_stopper_task_checks_position ON stopper_task_checks (task_id,product_id,COALESCE(doc_id,'00000000-0000-0000-0000-000000000000'));
CREATE INDEX idx_stopper_task_checks_user ON stopper_task_checks (user_id);
