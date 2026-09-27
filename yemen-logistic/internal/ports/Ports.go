// Package repository defines the hexagonal "ports" -- interfaces the domain
// and HTTP layers depend on, implemented concretely by internal/repository/postgres.
package repository

import (
	"context"
	"errors"

	"yemen-mit-platform/internal/domain"
)

var (
	ErrNotFound           = errors.New("repository: not found")
	ErrDuplicateUsername  = errors.New("repository: username already taken")
	ErrDuplicateLicense   = errors.New("repository: license number already registered")
	ErrInsufficientBudget = errors.New("repository: insufficient budget")
)

type MerchantRepository interface {
	Create(ctx context.Context, m domain.Merchant) (domain.Merchant, error)
	FindByUsername(ctx context.Context, username string) (domain.Merchant, error)
	FindByID(ctx context.Context, id string) (domain.Merchant, error)
	DeductBudget(ctx context.Context, merchantID string, amount float64) (newBudget float64, err error)
}

type PermitRepository interface {
	Create(ctx context.Context, p domain.Permit) (domain.Permit, error)
	FindByID(ctx context.Context, id string) (domain.Permit, error)
	ListByMerchant(ctx context.Context, merchantID string) ([]domain.Permit, error)
}