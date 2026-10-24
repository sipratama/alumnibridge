ALTER TABLE profiles ADD COLUMN moderation_status TEXT NOT NULL DEFAULT 'visible'
    CHECK (moderation_status IN ('visible','hidden'));
-- The old photo CHECK can evaluate to NULL. Require a complete metadata tuple.
ALTER TABLE profiles ADD CONSTRAINT profiles_photo_metadata_complete CHECK (
    (photo_object_key IS NULL AND photo_content_type IS NULL AND photo_size_bytes IS NULL)
    OR COALESCE(photo_object_key IS NOT NULL AND photo_content_type IS NOT NULL
        AND photo_size_bytes IS NOT NULL AND photo_content_type IN ('image/jpeg','image/png','image/webp')
        AND photo_size_bytes BETWEEN 1 AND 5242880, false)
);
CREATE TABLE profile_photo_objects (
    object_key TEXT PRIMARY KEY,
    owner_id UUID REFERENCES users(id) ON DELETE SET NULL,
    state TEXT NOT NULL CHECK (state IN ('staging','active','garbage')),
    content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg','image/png','image/webp')),
    size_bytes INTEGER NOT NULL CHECK (size_bytes BETWEEN 1 AND 5242880),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '15 minutes',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO profile_photo_objects(object_key,owner_id,state,content_type,size_bytes)
SELECT photo_object_key,user_id,'active',photo_content_type,photo_size_bytes FROM profiles WHERE photo_object_key IS NOT NULL;
ALTER TABLE profiles ADD CONSTRAINT profiles_photo_object_fkey FOREIGN KEY(photo_object_key)
    REFERENCES profile_photo_objects(object_key);
CREATE INDEX profile_photo_objects_cleanup_idx ON profile_photo_objects(state,expires_at);
CREATE UNIQUE INDEX profiles_photo_object_key_idx ON profiles(photo_object_key) WHERE photo_object_key IS NOT NULL;
CREATE INDEX profile_photo_objects_owner_idx ON profile_photo_objects(owner_id);
