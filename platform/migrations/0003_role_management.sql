ALTER TABLE roles
    ADD COLUMN IF NOT EXISTS description varchar(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS is_builtin boolean NOT NULL DEFAULT false;
