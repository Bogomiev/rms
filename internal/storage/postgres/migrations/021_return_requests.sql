CREATE TABLE return_request (
    id UUID PRIMARY KEY,
    date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    number TEXT,
    manager BIGINT NOT NULL REFERENCES users(id),
    store_id UUID NOT NULL REFERENCES stores(id),
    seller UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'NEW'
        CHECK (status IN ('NEW', 'SENT', 'PROCESSED', 'CANCELLED', 'COMPLETED'))
);
CREATE INDEX return_request_store_date_idx ON return_request (store_id, date DESC);

CREATE TABLE return_request_items (
    return_request_id UUID NOT NULL REFERENCES return_request(id) ON DELETE CASCADE,
    line SMALLINT NOT NULL CHECK (line > 0),
    product_id UUID NOT NULL REFERENCES products(id),
    amount NUMERIC NOT NULL CHECK (amount > 0),
    mark TEXT CHECK (mark IS NULL OR amount = 1)
);
CREATE INDEX return_request_items_request_idx ON return_request_items (return_request_id, line);
CREATE UNIQUE INDEX return_request_items_unmarked_idx ON return_request_items (return_request_id, line)
    WHERE mark IS NULL;
CREATE UNIQUE INDEX return_request_items_mark_idx ON return_request_items (return_request_id, mark)
    WHERE mark IS NOT NULL;

CREATE TABLE message_queue (
    id BIGSERIAL PRIMARY KEY,
    queue_name VARCHAR(100) NOT NULL DEFAULT 'default',
    task_handler VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status SMALLINT NOT NULL DEFAULT 0 CHECK (status BETWEEN 0 AND 3),
    priority SMALLINT NOT NULL DEFAULT 0,
    attempts SMALLINT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts SMALLINT NOT NULL DEFAULT 10 CHECK (max_attempts > 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX message_queue_pending_idx ON message_queue (queue_name, priority DESC, id)
    WHERE status = 0 AND attempts < max_attempts;
CREATE UNIQUE INDEX message_queue_active_return_idx ON message_queue ((payload->>'id'))
    WHERE status IN (0, 1) AND attempts < max_attempts
    AND task_handler = 'return_request';
