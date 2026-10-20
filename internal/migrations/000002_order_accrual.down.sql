DROP INDEX IF EXISTS orders_user_uploaded_idx;
ALTER TABLE orders DROP COLUMN IF EXISTS accrual;
