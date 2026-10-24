ALTER TABLE profiles
    ADD COLUMN directory_discoverable BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN directory_consented_at TIMESTAMPTZ,
    ADD CONSTRAINT profiles_directory_consent CHECK (
        (directory_discoverable AND directory_consented_at IS NOT NULL AND visibility = 'public')
        OR (NOT directory_discoverable AND directory_consented_at IS NULL)
    );

CREATE TABLE profile_directory_affiliations (
    membership_id UUID PRIMARY KEY REFERENCES memberships(id) ON DELETE CASCADE,
    show_institution BOOLEAN NOT NULL DEFAULT false,
    show_study_program BOOLEAN NOT NULL DEFAULT false,
    show_graduation_year BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (show_institution OR (NOT show_study_program AND NOT show_graduation_year))
);
CREATE INDEX profiles_directory_slug_idx ON profiles (slug COLLATE "C")
    WHERE visibility = 'public' AND moderation_status = 'visible' AND directory_discoverable;
CREATE INDEX memberships_directory_verified_idx ON memberships (user_id, study_program_id, graduation_year)
    WHERE role = 'alumni' AND status = 'verified';

-- All existing privacy writers inherit transactional withdrawal. No extra HTTP call.
CREATE FUNCTION withdraw_directory_consent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.visibility = 'private' THEN
        NEW.directory_discoverable := false;
    END IF;
    IF NOT NEW.directory_discoverable THEN
        NEW.directory_consented_at := NULL;
        DELETE FROM profile_directory_affiliations a USING memberships m
            WHERE a.membership_id = m.id AND m.user_id = NEW.user_id;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER profiles_directory_withdrawal BEFORE UPDATE OF visibility, directory_discoverable ON profiles
    FOR EACH ROW EXECUTE FUNCTION withdraw_directory_consent();

-- BEFORE triggers acquire the profile lock before any child FK cascade locks.
-- Consent writers lock memberships (UUID order), then profile, then affiliations.
CREATE FUNCTION revoke_membership_directory_consent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE loses_verification BOOLEAN;
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.user_id = OLD.user_id AND NEW.study_program_id = OLD.study_program_id
       AND NEW.role = OLD.role AND NEW.status = OLD.status
       AND NEW.graduation_year IS NOT DISTINCT FROM OLD.graduation_year THEN
        RETURN NEW;
    END IF;
    -- A write (not only a row lock) also rejects obsolete REPEATABLE READ snapshots.
    -- The lifecycle caller must retry the whole transaction on SQLSTATE 40001.
    -- During account deletion the parent is already gone; its profile cascades too.
    UPDATE profiles SET updated_at = now() WHERE user_id = OLD.user_id
        AND EXISTS (SELECT 1 FROM users WHERE id = OLD.user_id);
    DELETE FROM profile_directory_affiliations WHERE membership_id = OLD.id;
    IF TG_OP = 'DELETE' THEN
        loses_verification := true;
    ELSE
        loses_verification := NEW.user_id <> OLD.user_id OR NEW.role <> 'alumni' OR NEW.status <> 'verified';
    END IF;
    IF loses_verification AND OLD.role = 'alumni' AND OLD.status = 'verified' AND NOT EXISTS (
        SELECT 1 FROM memberships WHERE user_id = OLD.user_id AND id <> OLD.id
            AND role = 'alumni' AND status = 'verified'
    ) THEN
        UPDATE profiles SET directory_discoverable = false, directory_consented_at = NULL,
            updated_at = now() WHERE user_id = OLD.user_id;
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER memberships_directory_revocation BEFORE UPDATE OR DELETE ON memberships
    FOR EACH ROW EXECUTE FUNCTION revoke_membership_directory_consent();
