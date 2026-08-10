-- Bank-transfer checkout collects a payment-slip upload as proof of payment,
-- but the order record had nowhere to store the resulting file URL.

ALTER TABLE store_orders
    ADD COLUMN IF NOT EXISTS payment_slip_url TEXT;
