CREATE TYPE notification_channel AS ENUM ('sms', 'email', 'push');
CREATE TYPE notification_priority AS ENUM ('high', 'normal', 'low');
CREATE TYPE notification_status AS ENUM ('pending', 'processing', 'delivered', 'failed', 'cancelled');

CREATE TABLE notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id    UUID NOT NULL REFERENCES batches(id),
    recipient   VARCHAR(256) NOT NULL,
    channel     notification_channel NOT NULL,
    content     VARCHAR(1024) NOT NULL,
    priority    notification_priority NOT NULL DEFAULT 'normal',
    status      notification_status NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_batch_id ON notifications(batch_id);

CREATE INDEX idx_notifications_filter ON notifications(status, channel, created_at DESC);