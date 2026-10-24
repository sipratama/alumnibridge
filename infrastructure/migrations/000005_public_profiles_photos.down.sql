-- Keep pending cleanup intent; referenced photos remain recoverable from profile metadata.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM profile_photo_objects o WHERE NOT EXISTS
        (SELECT 1 FROM profiles p WHERE p.photo_object_key=o.object_key)) THEN
        RAISE EXCEPTION 'photo cleanup must complete before rollback';
    END IF;
END $$;
DROP INDEX profiles_photo_object_key_idx;
ALTER TABLE profiles DROP CONSTRAINT profiles_photo_object_fkey;
DROP TABLE profile_photo_objects;
ALTER TABLE profiles DROP CONSTRAINT profiles_photo_metadata_complete;
ALTER TABLE profiles DROP COLUMN moderation_status;
