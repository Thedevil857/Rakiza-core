-- =============================================================================
-- Migration: 000004_permit_checkpoint_clearances.down.sql
-- Reverses 000004_permit_checkpoint_clearances.up.sql.
-- =============================================================================

BEGIN;

DROP INDEX IF EXISTS idx_permit_clearances_decided_at;
DROP INDEX IF EXISTS idx_permit_clearances_officer_id;
DROP INDEX IF EXISTS idx_permit_clearances_permit_id;

DROP TABLE IF EXISTS permit_checkpoint_clearances;

DROP TYPE IF EXISTS permit_clearance_decision;

COMMIT;