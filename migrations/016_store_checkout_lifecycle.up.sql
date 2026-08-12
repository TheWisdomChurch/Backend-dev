-- Professional store checkout lifecycle: retry safety, private customer access,
-- independent payment state, proof tracking, and idempotent stock release.
ALTER TABLE store_orders
  ADD COLUMN IF NOT EXISTS payment_status varchar(30) NOT NULL DEFAULT 'unpaid',
  ADD COLUMN IF NOT EXISTS idempotency_key varchar(100),
  ADD COLUMN IF NOT EXISTS access_token_hash varchar(64),
  ADD COLUMN IF NOT EXISTS payment_slip_url text,
  ADD COLUMN IF NOT EXISTS paid_at timestamptz,
  ADD COLUMN IF NOT EXISTS cancelled_at timestamptz,
  ADD COLUMN IF NOT EXISTS stock_released_at timestamptz;

ALTER TABLE store_orders ADD COLUMN IF NOT EXISTS reservation_expires_at timestamptz;

UPDATE store_orders SET
  idempotency_key = COALESCE(idempotency_key, id::text),
  access_token_hash = COALESCE(access_token_hash, encode(digest(id::text || order_id, 'sha256'), 'hex'));

ALTER TABLE store_orders
  ALTER COLUMN idempotency_key SET NOT NULL,
  ALTER COLUMN access_token_hash SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_store_orders_idempotency_key ON store_orders(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_store_orders_payment_status ON store_orders(payment_status);
CREATE INDEX IF NOT EXISTS idx_store_orders_expiring_reservations ON store_orders(reservation_expires_at)
  WHERE status = 'pending' AND payment_status = 'unpaid' AND stock_released_at IS NULL;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_store_orders_status') THEN
    ALTER TABLE store_orders ADD CONSTRAINT chk_store_orders_status
      CHECK (status IN ('pending','processing','shipped','delivered','cancelled'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_store_orders_payment_status') THEN
    ALTER TABLE store_orders ADD CONSTRAINT chk_store_orders_payment_status
      CHECK (payment_status IN ('unpaid','proof_submitted','paid','failed','refunded'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_store_orders_payment_method') THEN
    ALTER TABLE store_orders ADD CONSTRAINT chk_store_orders_payment_method
      CHECK (payment_method IN ('transfer','delivery','online'));
  END IF;
END $$;
