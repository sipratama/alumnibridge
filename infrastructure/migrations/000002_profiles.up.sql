CREATE TABLE profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    slug TEXT NOT NULL UNIQUE,
    headline TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    current_company TEXT NOT NULL DEFAULT '',
    current_job_title TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
    show_bio BOOLEAN NOT NULL DEFAULT false,
    show_location BOOLEAN NOT NULL DEFAULT false,
    show_current_company BOOLEAN NOT NULL DEFAULT false,
    show_current_job_title BOOLEAN NOT NULL DEFAULT false,
    show_experiences BOOLEAN NOT NULL DEFAULT false,
    show_educations BOOLEAN NOT NULL DEFAULT false,
    show_skills BOOLEAN NOT NULL DEFAULT false,
    show_photo BOOLEAN NOT NULL DEFAULT false,
    photo_object_key TEXT,
    photo_content_type TEXT,
    photo_size_bytes INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) BETWEEN 3 AND 64),
    CHECK (length(headline) <= 160 AND length(bio) <= 5000 AND length(location) <= 120),
    CHECK (length(current_company) <= 160 AND length(current_job_title) <= 160),
    CHECK ((photo_object_key IS NULL AND photo_content_type IS NULL AND photo_size_bytes IS NULL)
        OR (photo_object_key IS NOT NULL AND photo_content_type IN ('image/jpeg', 'image/png', 'image/webp')
            AND photo_size_bytes BETWEEN 1 AND 5242880))
);

CREATE TABLE profile_slug_history (
    slug TEXT PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) BETWEEN 3 AND 64)
);
CREATE INDEX profile_slug_history_user_id_idx ON profile_slug_history(user_id);

CREATE TABLE profile_experiences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    company TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    start_date DATE NOT NULL,
    end_date DATE,
    is_current BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (length(trim(company)) BETWEEN 1 AND 160),
    CHECK (length(trim(title)) BETWEEN 1 AND 160),
    CHECK (length(description) <= 5000),
    CHECK (end_date IS NULL OR end_date >= start_date),
    CHECK (NOT is_current OR end_date IS NULL)
);
CREATE INDEX profile_experiences_user_order_idx ON profile_experiences(user_id, start_date DESC, id);

CREATE TABLE profile_educations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    institution_id UUID REFERENCES institutions(id),
    institution_name TEXT NOT NULL,
    degree TEXT NOT NULL DEFAULT '',
    field_of_study TEXT NOT NULL DEFAULT '',
    start_date DATE,
    end_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (length(trim(institution_name)) BETWEEN 1 AND 200),
    CHECK (length(degree) <= 160 AND length(field_of_study) <= 160),
    CHECK (end_date IS NULL OR start_date IS NULL OR end_date >= start_date)
);
CREATE INDEX profile_educations_user_order_idx ON profile_educations(user_id, start_date DESC NULLS LAST, id);

CREATE TABLE skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (length(trim(name)) BETWEEN 1 AND 80),
    CHECK (normalized_name = lower(trim(normalized_name)) AND length(normalized_name) BETWEEN 1 AND 80)
);

CREATE TABLE profile_skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL REFERENCES skills(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, skill_id)
);
CREATE INDEX profile_skills_user_id_idx ON profile_skills(user_id);
