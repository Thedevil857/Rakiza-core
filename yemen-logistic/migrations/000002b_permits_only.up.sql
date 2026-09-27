-- =============================================================================
-- Migration: 000002b_permits_only.up.sql
-- Creates ONLY the permits table and its enum. Does not touch `merchants`
-- at all -- that table already has every column it needs on this database,
-- just under its original names, so nothing here should conflict with it.
--
-- Uses IF NOT EXISTS / a DO block for the enum specifically so that if this
-- is run more than once, or after a partial prior failure, it does not
-- error out and silently skip the rest of the file the way a bare
-- CREATE TYPE / CREATE TABLE would.
-- =============================================================================

BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'permit_status') THEN
        CREATE TYPE permit_status AS ENUM (
            'Pending',
            'Cleared',
            'Rejected'
        );
    END IF;
END$$;

-- id is TEXT (not UUID) to match the human-readable "YEM-####" format the
-- frontend already displays and the QR endpoint already looks up by.
CREATE TABLE IF NOT EXISTS permits (
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

CREATE INDEX IF NOT EXISTS idx_permits_merchant_id ON permits (merchant_id);
CREATE INDEX IF NOT EXISTS idx_permits_status ON permits (status);

-- Guards against set_updated_at() not existing on this database (it should,
-- from 000001, but this session has had more than one surprise already) --
-- fails loudly and clearly here instead of silently skipping the trigger.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'set_updated_at') THEN
        RAISE EXCEPTION 'set_updated_at() function does not exist on this database -- check your 000001_init_schema.up.sql was fully applied';
    END IF;
END$$;

DROP TRIGGER IF EXISTS trg_permits_updated_at ON permits;
CREATE TRIGGER trg_permits_updated_at
    BEFORE UPDATE ON permits
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;