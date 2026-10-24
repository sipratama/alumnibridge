-- Development rollback only: this removes membership-driven ETag invalidation.
-- Public deployments require an operator-reviewed rollback plan before use.
DROP TRIGGER memberships_verified_insert_revision ON memberships;
DROP FUNCTION bump_inserted_verified_membership_revision();

-- Restore the exact 000006 behavior without changing migration history.
CREATE OR REPLACE FUNCTION revoke_membership_directory_consent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE loses_verification BOOLEAN;
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.user_id = OLD.user_id AND NEW.study_program_id = OLD.study_program_id
       AND NEW.role = OLD.role AND NEW.status = OLD.status
       AND NEW.graduation_year IS NOT DISTINCT FROM OLD.graduation_year THEN
        RETURN NEW;
    END IF;
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
