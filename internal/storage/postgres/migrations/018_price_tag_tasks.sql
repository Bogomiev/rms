-- «Печать ценников»: a standing store task (one per store, never closed) to print price tags for price changes.
-- The service price_print exists since 008; the type is added only when no such type exists yet.
INSERT INTO task_types (name,short_name,service,color,color_dark,sort_order)
SELECT 'Печать ценников','Ценники','price_print','#E6E3FB','#25223F',0
WHERE NOT EXISTS (SELECT 1 FROM task_types WHERE service='price_print')
ON CONFLICT (name) DO NOTHING;
-- Positions of a «Печать ценников» task, one per product. source tells why the tag is printed:
-- revaluation — a store price changed today in 1C (today_price_changes, migration 017), promo_start / promo_end — a stopper period starts / ended the day
-- before (doc_id is the promotion document), stopper_bad — marked «Не соответствует» in a «Стопперы» task,
-- manual — added in the task or from the product card. actual_at is when the latest reason appeared:
-- the position is printed while printed_at >= actual_at; a newer reason moves actual_at and the tag is
-- printed again. printed_at is the last print, prev_printed_at the print made before the position was put
-- to print again. unit is the unit of measure on the tag: pc (шт), 100g or kg for weighed goods.
-- tag_type: regular price, promo — promotional price with the regular one crossed out,
-- card — promotional price with a loyalty card and the regular price «без карты».
CREATE TABLE price_tag_task_items (
 id BIGSERIAL PRIMARY KEY,
 task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 product_id UUID NOT NULL,
 source TEXT NOT NULL CHECK (source IN ('revaluation','promo_start','promo_end','stopper_bad','manual')),
 doc_id UUID,
 tag_type TEXT NOT NULL CHECK (tag_type IN ('regular','promo','card')),
 quantity SMALLINT NOT NULL DEFAULT 1 CHECK (quantity BETWEEN 1 AND 99),
 actual_at TIMESTAMP NOT NULL,
 printed_at TIMESTAMP,
 prev_printed_at TIMESTAMP,
 unit TEXT NOT NULL DEFAULT 'pc' CHECK (unit IN ('pc','100g','kg')),
 printed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE (task_id, product_id)
);
CREATE INDEX idx_price_tag_task_items_printed_by ON price_tag_task_items (printed_by);
CREATE INDEX idx_price_tag_task_items_product ON price_tag_task_items (product_id);
