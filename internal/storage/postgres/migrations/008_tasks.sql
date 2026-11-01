CREATE TABLE task_types (
 id SMALLSERIAL PRIMARY KEY,
 name TEXT NOT NULL UNIQUE CHECK (length(trim(name)) > 0),
 short_name TEXT NOT NULL DEFAULT '',
 service TEXT NOT NULL CHECK (service IN ('cash_collection','price_print','without_service')),
 icon BYTEA,
 color TEXT NOT NULL DEFAULT '#2563eb' CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
 color_dark TEXT NOT NULL DEFAULT '#60a5fa' CHECK (color_dark ~ '^#[0-9a-fA-F]{6}$'),
 is_active BOOLEAN NOT NULL DEFAULT TRUE,
 sort_order SMALLINT NOT NULL DEFAULT 0,
 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE task_scheduling (
 id BIGSERIAL PRIMARY KEY,
 task_type_id SMALLINT NOT NULL REFERENCES task_types(id) ON DELETE RESTRICT,
 frequency SMALLINT NOT NULL CHECK (frequency BETWEEN 1 AND 10),
 period_from DATE NOT NULL CHECK (extract(isodow FROM period_from) = 1),
 period_to DATE NOT NULL CHECK (extract(isodow FROM period_to) = 7 AND period_to >= period_from),
 days SMALLINT[] NOT NULL CHECK (array_ndims(days) = 1 AND array_lower(days,1) = 1 AND cardinality(days) = frequency * 7 AND days <@ ARRAY[0,1]::smallint[] AND array_position(days,NULL) IS NULL),
 time TIME NOT NULL,
 is_active BOOLEAN NOT NULL DEFAULT TRUE,
 stores UUID[] NOT NULL DEFAULT '{}',
 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_task_scheduling_type ON task_scheduling(task_type_id);
CREATE INDEX idx_task_scheduling_stores ON task_scheduling USING GIN(stores);
-- PostgreSQL has no foreign keys for array elements. Validate UUIDs directly
-- and lock referenced store rows so concurrent deletion cannot orphan an array.
CREATE OR REPLACE FUNCTION validate_schedule_store_ids() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE store_uuid UUID;
BEGIN
 FOREACH store_uuid IN ARRAY NEW.stores LOOP
  PERFORM id FROM stores WHERE id = store_uuid FOR KEY SHARE;
  IF NOT FOUND THEN
   RAISE EXCEPTION 'Unknown store in task_scheduling.stores: %', store_uuid
    USING ERRCODE = '23503', CONSTRAINT = 'task_scheduling_store_ids_fk';
  END IF;
 END LOOP;
 RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER task_scheduling_store_ids_fk
 AFTER INSERT OR UPDATE ON task_scheduling
 FOR EACH ROW EXECUTE FUNCTION validate_schedule_store_ids();

CREATE OR REPLACE FUNCTION restrict_scheduled_store() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'UPDATE' AND NEW.id = OLD.id THEN
  RETURN NEW;
 END IF;
 IF EXISTS (SELECT 1 FROM task_scheduling WHERE stores @> ARRAY[OLD.id]) THEN
  RAISE EXCEPTION 'Store % is referenced by task_scheduling.stores', OLD.id
   USING ERRCODE = '23503', CONSTRAINT = 'task_scheduling_store_ids_fk';
 END IF;
 IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER stores_schedule_restrict BEFORE DELETE OR UPDATE OF id ON stores
 FOR EACH ROW EXECUTE FUNCTION restrict_scheduled_store();
CREATE TABLE tasks (
 id BIGSERIAL PRIMARY KEY,
 type_id SMALLINT NOT NULL REFERENCES task_types(id) ON DELETE RESTRICT,
 store_id UUID NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
 start TIMESTAMP NOT NULL,
 status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new','in_progress','completed','cancelled')),
 custom_fields JSONB NOT NULL DEFAULT '{}',
 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_tasks_store ON tasks(store_id);
CREATE INDEX idx_tasks_type ON tasks(type_id);
CREATE TABLE task_status_history (
 id BIGSERIAL PRIMARY KEY,
 period TIMESTAMP NOT NULL,
 task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 status TEXT NOT NULL CHECK (status IN ('new','in_progress','completed','cancelled'))
);
CREATE INDEX idx_task_history_task ON task_status_history(task_id);
CREATE INDEX idx_task_history_user ON task_status_history(user_id);
CREATE INDEX idx_task_history_period_task ON task_status_history(period,task_id);
CREATE OR REPLACE FUNCTION touch_task_update_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.update_at = CURRENT_TIMESTAMP; RETURN NEW; END $$;
CREATE TRIGGER task_types_update_at BEFORE UPDATE ON task_types FOR EACH ROW EXECUTE FUNCTION touch_task_update_at();
CREATE TRIGGER task_scheduling_update_at BEFORE UPDATE ON task_scheduling FOR EACH ROW EXECUTE FUNCTION touch_task_update_at();
CREATE TRIGGER tasks_update_at BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION touch_task_update_at();
