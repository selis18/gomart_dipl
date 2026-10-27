
BEGIN;
ALTER TABLE orders DROP CONSTRAINT orders_user_id_fkey;
ALTER TABLE withdrawals DROP CONSTRAINT withdrawals_user_id_fkey;
ALTER TABLE users ALTER COLUMN id TYPE VARCHAR(32),
    ALTER COLUMN login TYPE VARCHAR(255), ALTER COLUMN password TYPE VARCHAR(60);
ALTER TABLE orders ALTER COLUMN number TYPE VARCHAR(128),
    ALTER COLUMN user_id TYPE VARCHAR(32), ALTER COLUMN status DROP DEFAULT;
ALTER TABLE orders ALTER COLUMN status TYPE order_status USING status::text::order_status;
ALTER TABLE orders ALTER COLUMN status SET DEFAULT 'NEW';
ALTER TABLE withdrawals ALTER COLUMN order_number TYPE VARCHAR(128),
    ALTER COLUMN user_id TYPE VARCHAR(32);
ALTER TABLE orders ADD CONSTRAINT orders_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE withdrawals ADD CONSTRAINT withdrawals_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
COMMIT;
