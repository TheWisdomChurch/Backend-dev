CREATE TABLE IF NOT EXISTS new_member_workflows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id uuid NOT NULL UNIQUE REFERENCES form_submissions(id) ON DELETE CASCADE,
    stage varchar(40) NOT NULL DEFAULT 'new',
    assigned_owner_id uuid REFERENCES users(id) ON DELETE SET NULL,
    next_action_at timestamptz,
    escalation_status varchar(30) NOT NULL DEFAULT 'none',
    escalated_at timestamptz,
    completed_at timestamptz,
    last_contacted_at timestamptz,
    last_reminder_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT new_member_workflow_stage_check CHECK (stage IN ('new','contact_attempted','contacted','orientation_scheduled','orientation_completed','integrated','closed')),
    CONSTRAINT new_member_workflow_escalation_check CHECK (escalation_status IN ('none','due','escalated','resolved'))
);

CREATE INDEX IF NOT EXISTS idx_new_member_workflows_owner ON new_member_workflows(assigned_owner_id, stage);
CREATE INDEX IF NOT EXISTS idx_new_member_workflows_due ON new_member_workflows(next_action_at) WHERE completed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_new_member_workflows_escalation ON new_member_workflows(escalation_status, next_action_at);

CREATE TABLE IF NOT EXISTS new_member_contacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES new_member_workflows(id) ON DELETE CASCADE,
    channel varchar(30) NOT NULL,
    outcome varchar(50) NOT NULL,
    notes text,
    contacted_at timestamptz NOT NULL,
    created_by_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT new_member_contact_channel_check CHECK (channel IN ('phone','email','sms','whatsapp','in_person','other'))
);
CREATE INDEX IF NOT EXISTS idx_new_member_contacts_workflow ON new_member_contacts(workflow_id, contacted_at DESC);

CREATE TABLE IF NOT EXISTS new_member_workflow_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES new_member_workflows(id) ON DELETE CASCADE,
    event_type varchar(50) NOT NULL,
    from_stage varchar(40),
    to_stage varchar(40),
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_new_member_history_workflow ON new_member_workflow_history(workflow_id, created_at DESC);

INSERT INTO new_member_workflows (submission_id, next_action_at)
SELECT fs.id, fs.created_at + INTERVAL '1 day'
FROM form_submissions fs
JOIN forms f ON f.id = fs.form_id
WHERE fs.deleted_at IS NULL AND f.deleted_at IS NULL
  AND (
    LOWER(COALESCE(f.settings->>'submissionTarget', '')) = 'member'
    OR LOWER(COALESCE(f.slug, '')) = 'add-new-member'
    OR trim(regexp_replace(LOWER(COALESCE(f.title, '')), '[^a-z0-9]+', ' ', 'g')) = 'add new member'
  )
ON CONFLICT (submission_id) DO NOTHING;
