package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"yemen-mit-platform/internal/domain"
)

type PermitClearanceRepository struct {
	pool *pgxpool.Pool
}

func NewPermitClearanceRepository(pool *pgxpool.Pool) *PermitClearanceRepository {
	return &PermitClearanceRepository{pool: pool}
}

// Create inserts a single officer checkpoint decision. DecidedAt must be
// set by the caller to when the officer actually made the call (often
// offline, hours or days before this INSERT runs) -- id, synced_at, and
// created_at are all filled in by the database.
func (r *PermitClearanceRepository) Create(ctx context.Context, c domain.PermitClearance) (domain.PermitClearance, error) {
	const q = `
		INSERT INTO permit_checkpoint_clearances (permit_id, officer_id, decision, decided_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, synced_at, created_at
	`
	err := r.pool.QueryRow(ctx, q, c.PermitID, c.OfficerID, c.Decision, c.DecidedAt).
		Scan(&c.ID, &c.SyncedAt, &c.CreatedAt)
	if err != nil {
		return domain.PermitClearance{}, fmt.Errorf("inserting permit clearance: %w", err)
	}
	return c, nil
}