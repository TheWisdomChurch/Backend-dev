DROP INDEX IF EXISTS idx_store_orders_payment_status;
DROP INDEX IF EXISTS idx_store_orders_idempotency_key;
ALTER TABLE store_orders
  DROP CONSTRAINT IF EXISTS chk_store_orders_payment_method,
  DROP CONSTRAINT IF EXISTS chk_store_orders_payment_status,
  DROP CONSTRAINT IF EXISTS chk_store_orders_status,
  DROP COLUMN IF EXISTS reservation_expires_at,
  DROP COLUMN IF EXISTS stock_released_at,
  DROP COLUMN IF EXISTS cancelled_at,
  DROP COLUMN IF EXISTS paid_at,
  DROP COLUMN IF EXISTS payment_slip_url,
  DROP COLUMN IF EXISTS access_token_hash,
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS payment_status;
