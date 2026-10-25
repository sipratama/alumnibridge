DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE access_status <> 'active') OR
       EXISTS (SELECT 1 FROM identity_subject_blocks) THEN
        RAISE EXCEPTION 'cannot rollback while identity access restrictions exist';
    END IF;
END $$;
DROP TABLE identity_subject_blocks;
ALTER TABLE users DROP COLUMN access_status;
