-- Client Portal: add password_hash for email+password login

ALTER TABLE clients ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255) DEFAULT '';
