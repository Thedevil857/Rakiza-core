-- =============================================================================
-- Migration: 000004_permit_checkpoint_clearances.up.sql
-- Stores officer pass/reject decisions made at checkpoints. Deliberately
-- separate from permits.status: that column already means "customs duty
-- paid" (set to 'Cleared' at permit creation, before any physical
-- inspection happens) -- reusing it for "physically inspected and passed"
-- would silently overload an existing, already-relied-upon meaning.
-- permit_id and officer_id are NOT foreign keys: permits.id is a
-- human-readable TEXT id (YEM-####) and internal_staff.id is a plain
-- integer, and coupling this table's schema tightly to either of those
-- via FK constraints adds fragility for no real benefit here -- this table
-- is an append-only decision log, not a table anything else joins against
-- in a way that requires referential integrity enforcement.
-- =============================================================================

BEGIN;

CREATE TYPE permit_clearance_decision AS ENUM (
    'Passed',
    'Rejected'
);

CREATE TABLE permit_checkpoint_clearances (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    permit_id   TEXT NOT NULL,
    officer_id  TEXT NOT NULL,
    decision    permit_clearance_decision NOT NULL,
    -- decided_at is the moment the officer actually made the call at the
    -- checkpoint (set client-side, on the phone, offline), NOT when this
    -- row happens to be inserted -- those can be hours or days apart for
    -- an officer syncing after returning to signal.
    decided_at  TIMESTAMPTZ NOT NULL,
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_permit_clearances_permit_id ON permit_checkpoint_clearances (permit_id);
CREATE INDEX idx_permit_clearances_officer_id ON permit_checkpoint_clearances (officer_id);
CREATE INDEX idx_permit_clearances_decided_at ON permit_checkpoint_clearances (decided_at);

COMMIT;