CREATE TABLE IF NOT EXISTS withdrawals (
    order_number TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    sum NUMERIC NOT NULL CHECK (sum > 0),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS withdrawals_user_processed_idx ON withdrawals (user_id, processed_at DESC);
