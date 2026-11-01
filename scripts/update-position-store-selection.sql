BEGIN;

ALTER TABLE positions
 ADD COLUMN IF NOT EXISTS store_selection_available BOOLEAN NOT NULL DEFAULT FALSE,
 ADD COLUMN IF NOT EXISTS store_selection_required BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE positions
SET store_selection_available = TRUE,
    store_selection_required = (name <> 'admin');

COMMIT;
