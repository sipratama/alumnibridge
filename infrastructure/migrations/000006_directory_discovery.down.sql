DROP TRIGGER memberships_directory_revocation ON memberships;
DROP FUNCTION revoke_membership_directory_consent();
DROP TRIGGER profiles_directory_withdrawal ON profiles;
DROP FUNCTION withdraw_directory_consent();
DROP INDEX memberships_directory_verified_idx;
DROP INDEX profiles_directory_slug_idx;
DROP TABLE profile_directory_affiliations;
ALTER TABLE profiles DROP CONSTRAINT profiles_directory_consent,
    DROP COLUMN directory_consented_at, DROP COLUMN directory_discoverable;
