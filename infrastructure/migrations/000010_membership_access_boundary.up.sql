-- Raw trusted membership SQL must not mutate an inactive owner's profile,
-- consent, affiliations or privacy revision after an access-state commit.
-- The row being updated/deleted is already locked before a BEFORE trigger;
-- trusted callers should lock active users first to avoid lock inversion.
CREATE FUNCTION guard_membership_active_owner() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    old_owner UUID;
    new_owner UUID;
    checked_owner UUID;
    owner_status TEXT;
BEGIN
    IF TG_OP <> 'INSERT' THEN old_owner := OLD.user_id; END IF;
    IF TG_OP <> 'DELETE' THEN new_owner := NEW.user_id; END IF;

    -- A transfer checks both accounts in deterministic UUID order.
    FOR checked_owner IN
        SELECT DISTINCT owner_id FROM (VALUES (old_owner), (new_owner)) AS owners(owner_id)
        WHERE owner_id IS NOT NULL ORDER BY owner_id
    LOOP
        SELECT access_status INTO owner_status FROM users
        WHERE id = checked_owner FOR UPDATE;
        IF NOT FOUND THEN
            -- ON DELETE CASCADE runs after the parent user row is removed.
            IF TG_OP = 'DELETE' THEN CONTINUE; END IF;
            RAISE EXCEPTION 'membership owner unavailable' USING ERRCODE = '23514';
        END IF;
        IF owner_status <> 'active' THEN
            RAISE EXCEPTION 'membership owner unavailable' USING ERRCODE = '23514';
        END IF;
    END LOOP;

    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;

-- PostgreSQL executes triggers with the same timing/event in name order.
-- This runs before memberships_directory_revocation on UPDATE/DELETE.
CREATE TRIGGER memberships_active_owner_guard
    BEFORE INSERT OR UPDATE OR DELETE ON memberships
    FOR EACH ROW EXECUTE FUNCTION guard_membership_active_owner();
