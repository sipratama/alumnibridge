-- Preserve reservations after account deletion without retaining an identity reference.
ALTER TABLE profile_slug_history DROP CONSTRAINT profile_slug_history_user_id_fkey;
ALTER TABLE profile_slug_history ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE profile_slug_history ADD CONSTRAINT profile_slug_history_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE profile_slug_history ADD CONSTRAINT profile_slug_history_slug_owner_key UNIQUE (slug, user_id);
INSERT INTO profile_slug_history(slug,user_id)
SELECT slug,user_id FROM profiles ON CONFLICT (slug) DO NOTHING;
ALTER TABLE profiles ADD CONSTRAINT profiles_slug_owner_fkey
    FOREIGN KEY (slug,user_id) REFERENCES profile_slug_history(slug,user_id)
    DEFERRABLE INITIALLY DEFERRED;
CREATE FUNCTION protect_slug_reservation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'slug reservations cannot be deleted' USING ERRCODE = '23514';
    END IF;
    IF NEW.slug IS DISTINCT FROM OLD.slug OR
       (NEW.user_id IS DISTINCT FROM OLD.user_id AND NEW.user_id IS NOT NULL) THEN
        RAISE EXCEPTION 'slug reservations cannot be reassigned' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER protect_slug_reservation BEFORE UPDATE OR DELETE ON profile_slug_history
    FOR EACH ROW EXECUTE FUNCTION protect_slug_reservation();
