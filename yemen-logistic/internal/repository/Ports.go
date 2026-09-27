// Package repository defines the persistence-layer interfaces that the
// domain/handler layers depend on, plus the sentinel errors every
// implementation (currently: internal/repository/postgres) must return so
// callers can branch on outcome without importing a database driver.
//
// NOTE: This file is a reconstruction. It was not provided directly — its
// shape was inferred from how MerchantRepository, ErrNotFound,
// ErrDuplicateUsername, ErrDuplicateLicense, and ErrInsufficientBudget are
// actually referenced in handlers/auth.go and
// repository/postgres/merchant_repository.go. If your real file differs
// (different method set, different error names), reconcile against that —
// this version is what the rest of this pass's code (StaffRepository,
// CompleteOnboarding, the rewritten AuthHandler) is written against.
package repository

import (
	"context"
	"errors"

	"yemen-mit-platform/internal/domain"
)

var (
	ErrNotFound           = errors.New("repository: record not found")
	ErrDuplicateUsername  = errors.New("repository: username already registered")
	ErrDuplicateLicense   = errors.New("repository: license number already registered")
	ErrDuplicateCRN       = errors.New("repository: commercial register number already registered")
	ErrInsufficientBudget = errors.New("repository: insufficient wallet balance")
)

// MerchantRepository is the persistence port for merchant accounts.
type MerchantRepository interface {
	Create(ctx context.Context, m domain.Merchant) (domain.Merchant, error)
	FindByUsername(ctx context.Context, username string) (domain.Merchant, error)
	FindByID(ctx context.Context, id string) (domain.Merchant, error)
	DeductBudget(ctx context.Context, merchantID string, amount float64) (float64, error)
	CompleteOnboarding(ctx context.Context, merchantID string, input domain.MerchantOnboardingInput) (domain.Merchant, error)
	ListPendingReview(ctx context.Context) ([]domain.Merchant, error)
	ApproveMerchant(ctx context.Context, merchantID string) (domain.Merchant, error)
}

// StaffRepository is the persistence port for internal Ministry accounts
// (admin / officer), backed by the internal_staff table added in
// 000003_merchant_onboarding_and_staff.up.sql.
type StaffRepository interface {
	Create(ctx context.Context, s domain.Staff) (domain.Staff, error)
	FindByUsername(ctx context.Context, username string) (domain.Staff, error)
}

// PermitRepository is the persistence port for customs permits. Reconstructed
// from its actual call sites in handlers/permit.go (Create in
// PermitHandler.Create, FindByID in PermitHandler.GetQR) -- this file was
// never shown to me directly, so if the real permit_repository.go declares
// additional methods beyond these two, they need to be added here too or
// anything calling them through this interface won't compile.
type PermitRepository interface {
	Create(ctx context.Context, p domain.Permit) (domain.Permit, error)
	FindByID(ctx context.Context, id string) (domain.Permit, error)
}

// PermitClearanceRepository is the persistence port for officer checkpoint
// decisions (pass/reject), backed by the permit_checkpoint_clearances
// table added in 000004_permit_checkpoint_clearances.up.sql.
type PermitClearanceRepository interface {
	Create(ctx context.Context, c domain.PermitClearance) (domain.PermitClearance, error)
}