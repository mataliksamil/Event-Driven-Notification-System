CREATE TYPE batch_status AS ENUM ('accepted', 'processing', 'completed', 'partially_completed');

CREATE TABLE batches (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key UUID NOT NULL UNIQUE,
    status      batch_status NOT NULL DEFAULT 'accepted',
    total_count INT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);