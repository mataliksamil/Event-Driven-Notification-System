ALTER TABLE notifications
    ADD COLUMN error_message TEXT,
    ADD COLUMN retry_count INTEGER DEFAULT 0;