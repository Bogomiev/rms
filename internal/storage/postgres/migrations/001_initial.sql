CREATE TABLE IF NOT EXISTS users (
 id BIGSERIAL PRIMARY KEY,
 name VARCHAR(255) NOT NULL,
 user_token VARCHAR(255) NOT NULL UNIQUE CHECK (length(trim(user_token)) > 0),
 password VARCHAR(255) NOT NULL,
 is_admin BOOLEAN NOT NULL DEFAULT false,
 login_failures INTEGER NOT NULL DEFAULT 0 CHECK (login_failures >= 0),
 login_blocked_until TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS sessions (
 id VARCHAR(255) PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 refresh_token TEXT NOT NULL,
 is_revoked BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS products (
 id UUID PRIMARY KEY,
 code VARCHAR(11) NOT NULL,
 name VARCHAR(100) NOT NULL,
 parent_id UUID NOT NULL,
 marked BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMP NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
 marking_type TEXT NOT NULL,
 is_weight BOOLEAN NOT NULL DEFAULT false,
 is_thermal_mode BOOLEAN NOT NULL DEFAULT false,
 barcodes JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(barcodes) = 'array')
);
CREATE INDEX IF NOT EXISTS idx_product_code ON products(code);
CREATE INDEX IF NOT EXISTS idx_product_name ON products(name);
CREATE INDEX IF NOT EXISTS idx_product_parent ON products(parent_id);
CREATE TABLE IF NOT EXISTS stores (
 id UUID PRIMARY KEY,
 code TEXT NOT NULL,
 name TEXT NOT NULL,
 address VARCHAR(250) NOT NULL,
 created_at TIMESTAMP NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
