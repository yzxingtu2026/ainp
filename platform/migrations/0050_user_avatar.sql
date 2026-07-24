ALTER TABLE users
  ADD COLUMN IF NOT EXISTS avatar_attachment_id varchar(64);
