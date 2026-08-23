ALTER TABLE attachments ADD COLUMN IF NOT EXISTS uploader_kind varchar(128) NOT NULL DEFAULT 'ainp.account';
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS client_surface varchar(128) NOT NULL DEFAULT '';
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS audience varchar(255) NOT NULL DEFAULT '';
