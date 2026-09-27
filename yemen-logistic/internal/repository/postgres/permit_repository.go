package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"yemen-mit-platform/internal/domain"
	"yemen-mit-platform/internal/repository"
)

type PermitRepository struct {
	pool *pgxpool.Pool
}

func NewPermitRepository(pool *pgxpool.Pool) *PermitRepository {
	return &PermitRepository{pool: pool}
}

func (r *PermitRepository) Create(ctx context.Context, p domain.Permit) (domain.Permit, error) {
	const q = `
		INSERT INTO permits (id, merchant_id, type, weight, value, duty, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`
	err := r.pool.QueryRow(ctx, q, p.ID, p.MerchantID, p.Type, p.Weight, p.Value, p.Duty, p.Status).
		Scan(&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return domain.Permit{}, fmt.Errorf("inserting permit: %w", err)
	}
	return p, nil
}

func (r *PermitRepository) FindByID(ctx context.Context, id string) (domain.Permit, error) {
	const q = `
		SELECT id, merchant_id, type, weight, value, duty, status, created_at, updated_at
		FROM permits WHERE id = $1
	`
	var p domain.Permit
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&p.ID, &p.MerchantID, &p.Type, &p.Weight, &p.Value, &p.Duty, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Permit{}, repository.ErrNotFound
	}
	if err != nil {
		return domain.Permit{}, fmt.Errorf("querying permit by id: %w", err)
	}
	return p, nil
}

func (r *PermitRepository) ListByMerchant(ctx context.Context, merchantID string) ([]domain.Permit, error) {
	const q = `
		SELECT id, merchant_id, type, weight, value, duty, status, created_at, updated_at
		FROM permits WHERE merchant_id = $1 ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, q, merchantID)
	if err != nil {
		return nil, fmt.Errorf("listing permits by merchant: %w", err)
	}
	defer rows.Close()

	var permits []domain.Permit
	for rows.Next() {
		var p domain.Permit
		if err := rows.Scan(&p.ID, &p.MerchantID, &p.Type, &p.Weight, &p.Value, &p.Duty, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning permit row: %w", err)
		}
		permits = append(permits, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating permit rows: %w", err)
	}
	return permits, nil
}
