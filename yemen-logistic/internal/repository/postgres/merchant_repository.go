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

type MerchantRepository struct {
	pool *pgxpool.Pool
}

func NewMerchantRepository(pool *pgxpool.Pool) *MerchantRepository {
	return &MerchantRepository{pool: pool}
}

func (r *MerchantRepository) Create(ctx context.Context, m domain.Merchant) (domain.Merchant, error) {
	const q = `
		INSERT INTO merchants (
			username, password_hash, company_name, company_name_ar, company_name_en,
			commercial_register_no, license_number, phone, wallet_balance, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`
	// company_name (the legacy Phase 1 NOT NULL column) is populated from
	// company_name_en so this new registration flow satisfies the existing
	// constraint without requiring a breaking migration to drop it.
	err := r.pool.QueryRow(ctx, q,
		m.Username, m.PasswordHash, m.CompanyNameEn, m.CompanyNameAr, m.CompanyNameEn,
		m.CommercialRegisterNo, m.LicenseNumber, m.Phone, m.WalletBalance, domain.MerchantStatusActive,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			switch pgErr.ConstraintName {
			case "uq_merchants_username":
				return domain.Merchant{}, repository.ErrDuplicateUsername
			case "uq_merchants_license_number":
				return domain.Merchant{}, repository.ErrDuplicateLicense
			case "uq_merchants_crn":
				return domain.Merchant{}, repository.ErrDuplicateCRN
			}
		}
		return domain.Merchant{}, fmt.Errorf("inserting merchant: %w", err)
	}
	m.Status = domain.MerchantStatusActive
	return m, nil
}

func (r *MerchantRepository) FindByUsername(ctx context.Context, username string) (domain.Merchant, error) {
	return r.scanOne(ctx, merchantSelectQuery+" WHERE username = $1", username)
}

func (r *MerchantRepository) FindByID(ctx context.Context, id string) (domain.Merchant, error) {
	return r.scanOne(ctx, merchantSelectQuery+" WHERE id = $1", id)
}

// merchantSelectQuery is shared by every read path (FindByUsername,
// FindByID, CompleteOnboarding's RETURNING clause) so the column list and
// scan target list can never silently drift apart between call sites.
const merchantSelectQuery = `
	SELECT id, username, password_hash, company_name_ar, company_name_en,
	       commercial_register_no, license_number, phone, wallet_balance, status,
	       onboarding_completed, tax_id, authorized_signatory_name, authorized_signatory_id,
	       address_line, city, gps_lat, gps_lng, trade_nature, primary_goods_category,
	       default_country_of_origin, default_port_of_entry, default_exporter_name,
	       legal_documents, created_at, updated_at
	FROM merchants
`

func (r *MerchantRepository) scanOne(ctx context.Context, q string, arg any) (domain.Merchant, error) {
	m, err := scanMerchantRow(r.pool.QueryRow(ctx, q, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Merchant{}, repository.ErrNotFound
	}
	if err != nil {
		return domain.Merchant{}, fmt.Errorf("querying merchant: %w", err)
	}
	return m, nil
}

// rowScanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query),
// letting scanMerchantRow serve both a single-row lookup and a multi-row
// list without duplicating the column list twice.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanMerchantRow is the one place that knows how to turn a
// merchantSelectQuery row into a domain.Merchant. This is also the fix for
// the login bug reported after this session's onboarding-column changes:
// trade_nature is scanned into an intermediate *string, not directly into
// *domain.MerchantTradeNature. Scanning NULL straight into a pointer to a
// custom named string type is not a reliably supported pgx pattern; every
// account without onboarding data (i.e. every account that existed before
// this session, and every new account before it finishes onboarding) has
// trade_nature = NULL, so that direct scan was failing on every lookup --
// which FindByUsername's caller (loginMerchant) then reported as
// "invalid_credentials", indistinguishable from a wrong password.
func scanMerchantRow(scanner rowScanner) (domain.Merchant, error) {
	var m domain.Merchant
	var tradeNatureRaw *string
	err := scanner.Scan(
		&m.ID, &m.Username, &m.PasswordHash, &m.CompanyNameAr, &m.CompanyNameEn,
		&m.CommercialRegisterNo, &m.LicenseNumber, &m.Phone, &m.WalletBalance, &m.Status,
		&m.OnboardingCompleted, &m.TaxID, &m.AuthorizedSignatoryName, &m.AuthorizedSignatoryID,
		&m.AddressLine, &m.City, &m.GPSLat, &m.GPSLng, &tradeNatureRaw, &m.PrimaryGoodsCategory,
		&m.DefaultCountryOfOrigin, &m.DefaultPortOfEntry, &m.DefaultExporterName,
		&m.LegalDocuments, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return domain.Merchant{}, err
	}
	if tradeNatureRaw != nil {
		tn := domain.MerchantTradeNature(*tradeNatureRaw)
		m.TradeNature = &tn
	}
	return m, nil
}

// ListPendingReview returns every merchant who has completed onboarding
// and is awaiting ministerial review (status = 'Pending'), oldest
// submission first, so the Minister's queue is worked in order.
func (r *MerchantRepository) ListPendingReview(ctx context.Context) ([]domain.Merchant, error) {
	rows, err := r.pool.Query(ctx, merchantSelectQuery+" WHERE status = 'Pending' AND onboarding_completed = true ORDER BY updated_at ASC")
	if err != nil {
		return nil, fmt.Errorf("querying pending merchants: %w", err)
	}
	defer rows.Close()

	var merchants []domain.Merchant
	for rows.Next() {
		m, err := scanMerchantRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning pending merchant row: %w", err)
		}
		merchants = append(merchants, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pending merchants: %w", err)
	}
	return merchants, nil
}

// ApproveMerchant transitions a merchant from Pending to Active, unlocking
// full platform access. It only succeeds if the merchant is currently
// Pending -- an already-Active merchant is not silently re-approved, and a
// nonexistent ID is not silently ignored; both surface as ErrNotFound so
// the handler can report "nothing to approve" rather than a false success.
func (r *MerchantRepository) ApproveMerchant(ctx context.Context, merchantID string) (domain.Merchant, error) {
	const q = `
		UPDATE merchants
		SET status = 'Active'
		WHERE id = $1 AND status = 'Pending'
	`
	tag, err := r.pool.Exec(ctx, q, merchantID)
	if err != nil {
		return domain.Merchant{}, fmt.Errorf("approving merchant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.Merchant{}, repository.ErrNotFound
	}
	return r.FindByID(ctx, merchantID)
}

// DeductBudget performs the balance check and deduction as a single atomic
// UPDATE so two concurrent permit submissions from the same merchant cannot
// both read a stale "sufficient" balance and double-spend it.
//
// Name kept as DeductBudget (not renamed to DeductWalletBalance) despite the
// underlying column rename to wallet_balance: this method's only caller is
// the not-yet-built permit/customs-duty submission flow, and renaming it
// here risks silently breaking that call site if it already exists
// elsewhere in the codebase and wasn't shown to me. Confirm no caller
// depends on the old name before any future rename.
func (r *MerchantRepository) DeductBudget(ctx context.Context, merchantID string, amount float64) (float64, error) {
	const q = `
		UPDATE merchants
		SET wallet_balance = wallet_balance - $2
		WHERE id = $1 AND wallet_balance >= $2
		RETURNING wallet_balance
	`
	var newBalance float64
	err := r.pool.QueryRow(ctx, q, merchantID, amount).Scan(&newBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the merchant doesn't exist, or -- far more likely -- the
		// WHERE wallet_balance >= $2 predicate failed: insufficient funds.
		return 0, repository.ErrInsufficientBudget
	}
	if err != nil {
		return 0, fmt.Errorf("deducting merchant wallet balance: %w", err)
	}
	return newBalance, nil
}

// CompleteOnboarding persists the merchant's first-time onboarding wizard
// submission and flips onboarding_completed to true in the same statement,
// so a client can never observe a partially-written profile with the flag
// already set.
func (r *MerchantRepository) CompleteOnboarding(ctx context.Context, merchantID string, input domain.MerchantOnboardingInput) (domain.Merchant, error) {
	const q = `
		UPDATE merchants
		SET tax_id = $2,
		    authorized_signatory_name = $3,
		    authorized_signatory_id = $4,
		    address_line = $5,
		    city = $6,
		    gps_lat = $7,
		    gps_lng = $8,
		    trade_nature = $9,
		    primary_goods_category = $10,
		    default_country_of_origin = $11,
		    default_port_of_entry = $12,
		    default_exporter_name = $13,
		    commercial_register_no = $14,
		    phone = $15,
		    legal_documents = $16,
		    onboarding_completed = true,
		    status = 'Pending'
		WHERE id = $1
	`
	tag, err := r.pool.Exec(ctx, q,
		merchantID,
		input.TaxID,
		input.AuthorizedSignatoryName,
		input.AuthorizedSignatoryID,
		input.AddressLine,
		input.City,
		input.GPSLat,
		input.GPSLng,
		input.TradeNature,
		input.PrimaryGoodsCategory,
		input.DefaultCountryOfOrigin,
		input.DefaultPortOfEntry,
		input.DefaultExporterName,
		input.CommercialRegisterNo,
		input.Phone,
		input.LegalDocuments,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_merchants_crn" {
			return domain.Merchant{}, repository.ErrDuplicateCRN
		}
		return domain.Merchant{}, fmt.Errorf("completing merchant onboarding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.Merchant{}, repository.ErrNotFound
	}

	// Re-fetch rather than RETURNING everything inline: reuses the same
	// merchantSelectQuery/scanOne path as every other read, so this method
	// can never drift out of sync with what FindByID returns.
	return r.FindByID(ctx, merchantID)
}