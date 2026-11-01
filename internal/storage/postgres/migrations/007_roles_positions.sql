CREATE TABLE roles (
 id BIGSERIAL PRIMARY KEY,
 name VARCHAR(255) NOT NULL UNIQUE CHECK (length(trim(name)) > 0)
);
CREATE TABLE positions (
 id BIGSERIAL PRIMARY KEY,
 name VARCHAR(255) NOT NULL UNIQUE CHECK (length(trim(name)) > 0)
);
CREATE TABLE position_roles (
 position_id BIGINT NOT NULL REFERENCES positions(id) ON DELETE RESTRICT,
 role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
 PRIMARY KEY (position_id, role_id)
);
CREATE INDEX idx_position_roles_role ON position_roles(role_id);
ALTER TABLE users ADD COLUMN position BIGINT REFERENCES positions(id) ON DELETE RESTRICT;
CREATE INDEX idx_users_position ON users(position);
ALTER TABLE sessions DROP CONSTRAINT sessions_user_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;

INSERT INTO roles(name) VALUES ('admin'),('orders'),('products'),('documents'),('goods-receipts'),('goods-distribution');
INSERT INTO positions(name) VALUES ('admin'),('seller');
INSERT INTO position_roles(position_id,role_id)
SELECT p.id,r.id FROM positions p CROSS JOIN roles r
WHERE (p.name='admin' AND r.name='admin') OR (p.name='seller' AND r.name <> 'admin');
UPDATE users SET position=(SELECT id FROM positions WHERE name=CASE WHEN users.user_token='admin' THEN 'admin' ELSE 'seller' END),
 is_admin=(user_token='admin');

ALTER TABLE positions
 ADD COLUMN IF NOT EXISTS seller_selection_available BOOLEAN NOT NULL DEFAULT FALSE,
 ADD COLUMN IF NOT EXISTS store_selection_available BOOLEAN NOT NULL DEFAULT FALSE,
 ADD COLUMN IF NOT EXISTS store_selection_required BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE positions
SET store_selection_available = TRUE,
    store_selection_required = (name <> 'admin');
