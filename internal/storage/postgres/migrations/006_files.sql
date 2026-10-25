CREATE TABLE files (
 id UUID PRIMARY KEY,
 name TEXT NOT NULL CHECK (name <> '' AND name NOT IN ('.', '..') AND position('/' in name)=0),
 priority INTEGER NOT NULL DEFAULT 0,
 parent_id UUID REFERENCES files(id) ON DELETE RESTRICT,
 s3_key TEXT NOT NULL UNIQUE,
 is_file BOOLEAN NOT NULL,
 icon BYTEA,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 deleted_at TIMESTAMPTZ,
 CHECK (parent_id IS DISTINCT FROM id)
);
CREATE UNIQUE INDEX files_siblings ON files(COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), name);
CREATE INDEX files_parent_priority ON files(parent_id, priority, name, id);
CREATE FUNCTION files_validate_tree() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'UPDATE' AND NEW.is_file <> OLD.is_file THEN
  RAISE EXCEPTION 'is_file cannot be changed' USING ERRCODE = '23514';
 END IF;
 IF NEW.parent_id IS NOT NULL THEN
  PERFORM id FROM files WHERE id=NEW.parent_id AND NOT is_file AND deleted_at IS NULL FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'parent must be an active directory' USING ERRCODE = '23514'; END IF;
  IF EXISTS (WITH RECURSIVE ancestors AS (
   SELECT id,parent_id FROM files WHERE id=NEW.parent_id
   UNION SELECT f.id,f.parent_id FROM files f JOIN ancestors a ON f.id=a.parent_id
  ) SELECT 1 FROM ancestors WHERE id=NEW.id) THEN
   RAISE EXCEPTION 'directory cycle' USING ERRCODE = '23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER files_validate_tree BEFORE INSERT OR UPDATE ON files FOR EACH ROW EXECUTE FUNCTION files_validate_tree();
