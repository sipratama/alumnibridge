-- A membership transition can change the owner Discovery representation even
-- when another verified alumni membership keeps directory consent active.
-- Replace the 000006 trigger body without changing an applied migration.
CREATE OR REPLACE FUNCTION revoke_membership_directory_consent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    new_owner UUID;
    affected_owner UUID;
    old_verified BOOLEAN;
    new_verified BOOLEAN := false;
    revoke_old BOOLEAN;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.user_id = OLD.user_id AND NEW.study_program_id = OLD.study_program_id
           AND NEW.role = OLD.role AND NEW.status = OLD.status
           AND NEW.graduation_year IS NOT DISTINCT FROM OLD.graduation_year THEN
            RETURN NEW;
        END IF;
        new_owner := NEW.user_id;
        new_verified := NEW.role = 'alumni' AND NEW.status = 'verified';
    END IF;

    old_verified := OLD.role = 'alumni' AND OLD.status = 'verified';
    -- The membership row is already locked. Lock affected profiles in UUID
    -- order, then remove the old affiliation. A single profile UPDATE per owner
    -- avoids double increments when the last verified membership is revoked.
    FOR affected_owner IN
        SELECT DISTINCT owner_id FROM (VALUES (OLD.user_id), (new_owner)) AS owners(owner_id)
        WHERE owner_id IS NOT NULL ORDER BY owner_id
    LOOP
        IF affected_owner = OLD.user_id THEN
            -- Wait for the owner's profile before checking remaining memberships.
            -- READ COMMITTED takes a fresh snapshot for this query after the
            -- wait, so simultaneous revocations cannot each see the other row
            -- as verified and leave consent enabled.
            PERFORM 1 FROM profiles WHERE user_id = affected_owner FOR UPDATE;
            revoke_old := old_verified AND
                (TG_OP = 'DELETE' OR new_owner <> OLD.user_id OR NOT new_verified)
                AND NOT EXISTS (
                    SELECT 1 FROM memberships WHERE user_id = OLD.user_id AND id <> OLD.id
                        AND role = 'alumni' AND status = 'verified'
                );
            IF revoke_old THEN
                UPDATE profiles SET directory_discoverable = false,
                    directory_consented_at = NULL,
                    privacy_revision = privacy_revision + 1,
                    updated_at = now()
                WHERE user_id = affected_owner
                    AND EXISTS (SELECT 1 FROM users WHERE id = affected_owner);
            ELSE
                -- Keep the profile write for old-snapshot serialization even
                -- when an unverified membership change has no visible effect.
                UPDATE profiles SET
                    privacy_revision = privacy_revision + CASE
                        WHEN old_verified OR (new_verified AND new_owner = OLD.user_id) THEN 1
                        ELSE 0 END,
                    updated_at = now()
                WHERE user_id = affected_owner
                    AND EXISTS (SELECT 1 FROM users WHERE id = affected_owner);
            END IF;
        ELSIF new_verified THEN
            UPDATE profiles SET privacy_revision = privacy_revision + 1,
                updated_at = now()
            WHERE user_id = affected_owner;
        END IF;
    END LOOP;

    DELETE FROM profile_directory_affiliations WHERE membership_id = OLD.id;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;

-- A newly inserted verified membership also changes eligibility and the owner
-- Discovery choices. Inserts during onboarding may precede profile creation;
-- there is no earlier owner ETag to invalidate in that case.
CREATE FUNCTION bump_inserted_verified_membership_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.role = 'alumni' AND NEW.status = 'verified' THEN
        UPDATE profiles SET privacy_revision = privacy_revision + 1,
            updated_at = now()
        WHERE user_id = NEW.user_id;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER memberships_verified_insert_revision
    AFTER INSERT ON memberships
    FOR EACH ROW EXECUTE FUNCTION bump_inserted_verified_membership_revision();
