BEGIN;

DROP TRIGGER IF EXISTS trg_permits_updated_at ON permits;
DROP TABLE IF EXISTS permits;
DROP TYPE IF EXISTS permit_status;

ALTER TABLE merchants
    DROP CONSTRAINT IF EXISTS uq_merchants_username,
    DROP CONSTRAINT IF EXISTS uq_merchants_license_number;

DROP INDEX IF EXISTS idx_merchants_username;

ALTER TABLE merchants
    DROP COLUMN IF EXISTS username,
    DROP COLUMN IF EXISTS password_hash,
    DROP COLUMN IF EXISTS company_name_ar,
    DROP COLUMN IF EXISTS company_name_en,
    DROP COLUMN IF EXISTS license_number,
    DROP COLUMN IF EXISTS budget;

COMMIT;