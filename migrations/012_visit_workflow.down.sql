-- Destructive manual rollback: removes the visit workflow and all visit data.
-- The application migration runner never executes down migrations automatically.
DROP TABLE IF EXISTS visit_requests;
