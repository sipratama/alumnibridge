CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oidc_subject TEXT NOT NULL UNIQUE,
    email TEXT,
    display_name TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_oidc_subject_nonempty CHECK (length(oidc_subject) > 0)
);

CREATE TABLE institutions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);

CREATE TABLE study_programs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    institution_id UUID NOT NULL REFERENCES institutions(id),
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    UNIQUE (institution_id, slug)
);

CREATE TABLE memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    study_program_id UUID NOT NULL REFERENCES study_programs(id),
    role TEXT NOT NULL CHECK (role IN ('student', 'alumni')),
    status TEXT NOT NULL CHECK (status IN ('active', 'pending', 'verified', 'rejected')),
    graduation_year INTEGER CHECK (graduation_year BETWEEN 1950 AND 2100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, study_program_id, role),
    CHECK ((role = 'student' AND status = 'active' AND graduation_year IS NULL)
        OR (role = 'alumni' AND status IN ('pending', 'verified', 'rejected') AND graduation_year IS NOT NULL))
);

CREATE INDEX memberships_user_id_idx ON memberships(user_id);

INSERT INTO institutions (slug, name) VALUES ('uhamka', 'Universitas Muhammadiyah Prof. Dr. HAMKA');
INSERT INTO study_programs (institution_id, slug, name)
SELECT id, 'mathematics-education', 'Mathematics Education' FROM institutions WHERE slug = 'uhamka';
