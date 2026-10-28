ALTER TABLE orders ADD COLUMN IF NOT EXISTS accrual NUMERIC NULL;
CREATE INDEX IF NOT EXISTS orders_user_uploaded_idx ON orders (user_id, uploaded_at DESC, number DESC);
