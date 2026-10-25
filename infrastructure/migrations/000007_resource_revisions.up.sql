-- Issue #11: additive revision storage; conditional HTTP writes are wired in later phases.
-- Existing 000001-000006 migrations remain immutable.
ALTER TABLE profiles
    ADD COLUMN content_revision BIGINT NOT NULL DEFAULT 1 CHECK (content_revision > 0),
    ADD COLUMN privacy_revision BIGINT NOT NULL DEFAULT 1 CHECK (privacy_revision > 0);

ALTER TABLE profile_experiences
    ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0);

ALTER TABLE profile_educations
    ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0);

-- Automatically advance revisions for changes performed by any SQL writer,
-- including legacy callers, so conditional updates do not depend on updated_at.
-- A publication/consent edit that only changes affiliation rows MUST explicitly
-- increment privacy_revision in the enclosing profile transaction.
CREATE FUNCTION bump_profile_resource_revisions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.slug, NEW.headline, NEW.bio, NEW.location,
           NEW.current_company, NEW.current_job_title,
           NEW.photo_object_key, NEW.photo_content_type, NEW.photo_size_bytes)
       IS DISTINCT FROM
       ROW(OLD.slug, OLD.headline, OLD.bio, OLD.location,
           OLD.current_company, OLD.current_job_title,
           OLD.photo_object_key, OLD.photo_content_type, OLD.photo_size_bytes)
       AND NEW.content_revision = OLD.content_revision THEN
        NEW.content_revision := OLD.content_revision + 1;
    END IF;

    IF ROW(NEW.visibility, NEW.moderation_status,
           NEW.show_bio, NEW.show_location, NEW.show_current_company,
           NEW.show_current_job_title, NEW.show_experiences,
           NEW.show_educations, NEW.show_skills, NEW.show_photo,
           NEW.directory_discoverable, NEW.directory_consented_at)
       IS DISTINCT FROM
       ROW(OLD.visibility, OLD.moderation_status,
           OLD.show_bio, OLD.show_location, OLD.show_current_company,
           OLD.show_current_job_title, OLD.show_experiences,
           OLD.show_educations, OLD.show_skills, OLD.show_photo,
           OLD.directory_discoverable, OLD.directory_consented_at)
       AND NEW.privacy_revision = OLD.privacy_revision THEN
        NEW.privacy_revision := OLD.privacy_revision + 1;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER profiles_resource_revisions
    BEFORE UPDATE ON profiles
    FOR EACH ROW EXECUTE FUNCTION bump_profile_resource_revisions();

-- Child PATCH writes use owner-scoped row updates; the trigger advances the
-- per-item validator exactly once on any successful UPDATE. DELETE must use
-- the expected revision predicate in its SQL statement (later phase).
CREATE FUNCTION bump_profile_item_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.revision := OLD.revision + 1;
    RETURN NEW;
END;
$$;

CREATE TRIGGER profile_experiences_revision
    BEFORE UPDATE ON profile_experiences
    FOR EACH ROW EXECUTE FUNCTION bump_profile_item_revision();

CREATE TRIGGER profile_educations_revision
    BEFORE UPDATE ON profile_educations
    FOR EACH ROW EXECUTE FUNCTION bump_profile_item_revision();
