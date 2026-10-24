-- Refuse rollback if it would discard tombstones or reintroduce reusable slugs.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM profile_slug_history WHERE user_id IS NULL) THEN
        RAISE EXCEPTION 'cannot rollback while deleted-account slug tombstones exist';
    END IF;
END $$;
ALTER TABLE profiles DROP CONSTRAINT profiles_slug_owner_fkey;
DROP TRIGGER protect_slug_reservation ON profile_slug_history;
DROP FUNCTION protect_slug_reservation();
ALTER TABLE profile_slug_history DROP CONSTRAINT profile_slug_history_slug_owner_key;
ALTER TABLE profile_slug_history DROP CONSTRAINT profile_slug_history_user_id_fkey;
ALTER TABLE profile_slug_history ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE profile_slug_history ADD CONSTRAINT profile_slug_history_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
