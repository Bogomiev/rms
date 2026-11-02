ALTER TABLE task_types ADD COLUMN custom_fields JSONB NOT NULL DEFAULT '{}';
CREATE INDEX idx_tasks_current ON tasks(store_id,start) WHERE status IN ('new','in_progress');
