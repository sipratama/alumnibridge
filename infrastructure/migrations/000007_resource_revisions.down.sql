-- Development rollback only. Downgrading in a public deployment would
-- remove the atomic stale-write guarantees and requires an operator-approved plan.
DROP TRIGGER profile_educations_revision ON profile_educations;
DROP TRIGGER profile_experiences_revision ON profile_experiences;
DROP FUNCTION bump_profile_item_revision();
DROP TRIGGER profiles_resource_revisions ON profiles;
DROP FUNCTION bump_profile_resource_revisions();

ALTER TABLE profile_educations DROP COLUMN revision;
ALTER TABLE profile_experiences DROP COLUMN revision;
ALTER TABLE profiles DROP COLUMN privacy_revision, DROP COLUMN content_revision;
