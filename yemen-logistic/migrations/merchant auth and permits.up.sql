-- =============================================================================
-- Migration: 000002_add_merchant_auth_and_permits.up.sql
-- Adds merchant authentication fields + wallet budget, and a `permits` table
-- for the Merchant Portal's cargo submission flow.
-- =============================================================================

BEGIN;

ALTER TABLE merchants
    ADD COLUMN username        VARCHAR(100),
    ADD COLUMN password_hash   TEXT,
    ADD COLUMN company_name_ar VARCHAR(255),
    ADD COLUMN company_name_en VARCHAR(255),
    ADD COLUMN license_number  VARCHAR(100),
    ADD COLUMN budget          NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (budget >= 0);

-- Backfill company_name_ar from the legacy company_name column so any
-- existing Phase 1 rows remain valid before we tighten the new columns
-- to NOT NULL below.
UPDATE merchants SET company_name_ar = company_name WHERE company_name_ar IS NULL;

ALTER TABLE merchants
    ALTER COLUMN username        SET NOT NULL,
    ALTER COLUMN password_hash   SET NOT NULL,
    ALTER COLUMN company_name_ar SET NOT NULL,
    ALTER COLUMN license_number  SET NOT NULL;

ALTER TABLE merchants
    ADD CONSTRAINT uq_merchants_username       UNIQUE (username),
    ADD CONSTRAINT uq_merchants_license_number  UNIQUE (license_number);

CREATE INDEX idx_merchants_username ON merchants (username);

-- Mirrors the frontend's "pending"/"cleared" values, plus a rejection state
-- for insufficient-budget or manual-review outcomes.
CREATE TYPE permit_status AS ENUM (
    'Pending',
    'Cleared',
    'Rejected'
);

-- id is TEXT (not UUID) to match the human-readable "YEM-####" format the
-- frontend already displays and the QR endpoint already looks up by.
CREATE TABLE permits (
    id           TEXT PRIMARY KEY,
    merchant_id  UUID NOT NULL REFERENCES merchants (id) ON DELETE RESTRICT,
    type         VARCHAR(255) NOT NULL,
    weight       NUMERIC(12, 2) NOT NULL CHECK (weight > 0),
    value        NUMERIC(14, 2) NOT NULL CHECK (value > 0),
    duty         NUMERIC(14, 2) NOT NULL CHECK (duty >= 0),
    status       permit_status NOT NULL DEFAULT 'Pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_permits_merchant_id ON permits (merchant_id);
CREATE INDEX idx_permits_status ON permits (status);

CREATE TRIGGER trg_permits_updated_at
    BEFORE UPDATE ON permits
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;