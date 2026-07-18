-- Adds the stated reason a requester gives for an approval request — needed
-- for delete-approval flows (event/workforce/leadership deletion) so the
-- super-admin reviewing the request has actual context instead of just an
-- entity label. Nullable: existing rows and non-delete request types don't
-- require one.
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS reason TEXT;
