package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"yemen-mit-platform/internal/domain"
	"yemen-mit-platform/internal/repository"
)

type StaffRepository struct {
	pool *pgxpool.Pool
}

func NewStaffRepository(pool *pgxpool.Pool) *StaffRepository {
	return &StaffRepository{pool: pool}
}

// Create inserts a new internal_staff row. id is a plain Postgres integer
// (auto-generated, likely SERIAL), not a UUID -- this casts it to text on
// the way back out (RETURNING id::text) so domain.Staff.ID can stay a
// plain string, consistent with how merchant/JWT-subject IDs are handled
// everywhere else, without every caller needing to special-case the fact
// that internal_staff happens to use an integer PK while merchants uses a
// UUID.
func (r *StaffRepository) Create(ctx context.Context, s domain.Staff) (domain.Staff, error) {
	const q = `
		INSERT INTO internal_staff (username, password, role, name_ar, name_en, title_ar, title_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text
	`
	err := r.pool.QueryRow(ctx, q,
		s.Username, s.PasswordHash, s.Role, s.NameAr, s.NameEn, s.TitleAr, s.TitleEn,
	).Scan(&s.ID)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation, whatever the real constraint is named
			return domain.Staff{}, repository.ErrDuplicateUsername
		}
		return domain.Staff{}, fmt.Errorf("inserting staff account: %w", err)
	}
	return s, nil
}

func (r *StaffRepository) FindByUsername(ctx context.Context, username string) (domain.Staff, error) {
	const q = `
		SELECT id::text, username, password, role, name_ar, name_en, title_ar, title_en
		FROM internal_staff
		WHERE username = $1
	`
	var s domain.Staff
	err := r.pool.QueryRow(ctx, q, username).Scan(
		&s.ID, &s.Username, &s.PasswordHash, &s.Role, &s.NameAr, &s.NameEn, &s.TitleAr, &s.TitleEn,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Staff{}, repository.ErrNotFound
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("querying staff account: %w", err)
	}
	return s, nil
}