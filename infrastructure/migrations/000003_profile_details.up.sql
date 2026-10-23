ALTER TABLE profile_experiences
    ADD COLUMN employment_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN location TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT profile_experiences_employment_type_length CHECK (length(employment_type) <= 80),
    ADD CONSTRAINT profile_experiences_location_length CHECK (length(location) <= 120);

ALTER TABLE profile_educations
    ADD COLUMN description TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT profile_educations_description_length CHECK (length(description) <= 5000);
