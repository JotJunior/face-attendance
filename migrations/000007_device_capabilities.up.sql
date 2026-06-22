-- Migration 000007: add device capability columns (max_users, max_faces).
-- Ref: data-model.md §Entity Device, tasks.md §2.1
-- Columns are nullable (NULL until first ISAPI GetCapabilities call populates them).
ALTER TABLE devices
  ADD COLUMN max_users INTEGER NULL,
  ADD COLUMN max_faces INTEGER NULL;
