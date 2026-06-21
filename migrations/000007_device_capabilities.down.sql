-- Rollback migration 000007.
ALTER TABLE devices
  DROP COLUMN max_users,
  DROP COLUMN max_faces;
