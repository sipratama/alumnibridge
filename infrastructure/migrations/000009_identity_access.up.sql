ALTER TABLE users ADD COLUMN access_status TEXT NOT NULL DEFAULT 'active'
    CHECK (access_status IN ('active', 'suspended', 'deleting', 'deleted'));

-- Future deletion work may insert a block before removing a user row. This
-- slice neither inserts production blocks nor chooses their retention period.
CREATE TABLE identity_subject_blocks (
    issuer TEXT NOT NULL CHECK (length(issuer) > 0),
    subject TEXT NOT NULL CHECK (length(subject) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (issuer, subject)
);
