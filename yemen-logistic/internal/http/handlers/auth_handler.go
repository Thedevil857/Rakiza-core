package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"yemen-mit-platform/internal/auth"
	"yemen-mit-platform/internal/domain"
	"yemen-mit-platform/internal/repository"
)

type AuthHandler struct {
	merchants repository.MerchantRepository
	staff     repository.StaffRepository
	auth      *auth.Service
}

func NewAuthHandler(merchants repository.MerchantRepository, staff repository.StaffRepository, authSvc *auth.Service) *AuthHandler {
	return &AuthHandler{merchants: merchants, staff: staff, auth: authSvc}
}

type registerRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	CompanyNameAr string `json:"company_name_ar"`
	CompanyNameEn string `json:"company_name_en"`
	LicenseNumber string `json:"license_number"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	if req.Username == "" || req.Password == "" || req.CompanyNameAr == "" ||
		req.CompanyNameEn == "" || req.LicenseNumber == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_fields"})
		return
	}
	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
		return
	}

	hash, err := h.auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "hash_failed"})
		return
	}

	merchant := domain.Merchant{
		Username:      req.Username,
		PasswordHash:  hash,
		CompanyNameAr: req.CompanyNameAr,
		CompanyNameEn: req.CompanyNameEn,
		LicenseNumber: req.LicenseNumber,
		// CommercialRegisterNo/Phone are seeded from LicenseNumber/"N/A" as
		// placeholders here -- this is unchanged from Phase 1 and remains a
		// deliberate, temporary gap: the merchant onboarding wizard
		// (MerchantOnboardingHandler.Complete) overwrites both with real
		// values as its very first required fields, before
		// onboarding_completed can ever be set true. A merchant cannot
		// reach their dashboard with the placeholder values still in place.
		CommercialRegisterNo: req.LicenseNumber,
		Phone:                "N/A",
		WalletBalance:        0,
	}

	created, err := h.merchants.Create(r.Context(), merchant)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrDuplicateUsername):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username_taken"})
		case errors.Is(err, repository.ErrDuplicateLicense):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "license_already_registered"})
		case errors.Is(err, repository.ErrDuplicateCRN):
			// Reachable only if a prior merchant's real CRN happens to
			// equal this registrant's license number -- extremely
			// unlikely, but handled rather than falling through to a
			// generic 500.
			writeJSON(w, http.StatusConflict, map[string]string{"error": "license_already_registered"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration_failed"})
		}
		return
	}

	// Issue a session immediately on the newly created merchant row instead
	// of requiring the frontend to make a second /auth/login call. That
	// second call was the actual source of the 401: it re-ran
	// bcrypt.CompareHashAndPassword against whatever the client happened to
	// send moments later, over a second network round-trip -- a second
	// point of failure that had nothing to do with hashing correctness
	// itself (e.g. a password input that had already been cleared or
	// re-rendered by the time the second call fired). Registration and
	// session issuance are now atomic from the client's point of view: one
	// request, one response, no window for client-side state to drift.
	resp, err := h.issueMerchantSession(created)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_generation_failed"})
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type userProfile struct {
	ID                  string  `json:"id"`
	Role                string  `json:"role"`
	NameAr              string  `json:"nameAr"`
	NameEn              string  `json:"nameEn"`
	TitleAr             string  `json:"titleAr"`
	TitleEn             string  `json:"titleEn"`
	Initials            string  `json:"initials"`
	WalletBalance       float64 `json:"walletBalance"`
	OnboardingCompleted bool    `json:"onboardingCompleted"`
	// Status carries MerchantStatus ("Active"/"Pending"/"Suspended") for
	// merchants, or StaffStatus ("Active"/"Suspended") for staff. The
	// frontend's login flow checks this to show a "your account is under
	// review" message for merchants awaiting admin approval instead of
	// dropping them into the dashboard.
	Status string `json:"status"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  userProfile `json:"user"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing_credentials"})
		return
	}

	switch req.Role {
	case "merchant":
		h.loginMerchant(w, r, req)
	case "admin", "officer":
		h.loginStaff(w, r, req)
	default:
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "role_mismatch"})
	}
}

func (h *AuthHandler) loginMerchant(w http.ResponseWriter, r *http.Request, req loginRequest) {
	merchant, err := h.merchants.FindByUsername(r.Context(), req.Username)
	if err != nil {
		// Same generic error whether the username doesn't exist or the
		// lookup failed, to avoid username enumeration.
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	if !h.auth.VerifyPassword(merchant.PasswordHash, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}

	resp, err := h.issueMerchantSession(merchant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_generation_failed"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// issueMerchantSession builds the token + profile response shared by both
// loginMerchant and Register. Extracting this into one place is what
// guarantees the two code paths can't drift apart the way "register, then
// separately log in" could -- there is now exactly one place that decides
// what a merchant session response looks like.
func (h *AuthHandler) issueMerchantSession(merchant domain.Merchant) (loginResponse, error) {
	token, err := h.auth.IssueToken(merchant.ID, "merchant")
	if err != nil {
		return loginResponse{}, err
	}

	initials := "MC"
	if runes := []rune(merchant.CompanyNameEn); len(runes) >= 2 {
		initials = string(runes[:2])
	}

	return loginResponse{
		Token: token,
		User: userProfile{
			ID: merchant.ID, Role: "merchant",
			NameAr: merchant.CompanyNameAr, NameEn: merchant.CompanyNameEn,
			TitleAr: "مستورد مرخّص", TitleEn: "Licensed Importer",
			Initials:            initials,
			WalletBalance:       merchant.WalletBalance,
			OnboardingCompleted: merchant.OnboardingCompleted,
			Status:              string(merchant.Status),
		},
	}, nil
}

// loginStaff authenticates against the real internal_staff table. This
// replaces the Phase 1 legacyStaffProfiles map, which matched by role alone
// and never checked a password at all -- any request with role=admin or
// role=officer was issued a valid session token regardless of username or
// password. That gap is closed here: a bcrypt comparison is mandatory, and
// the account's actual stored role must match the role the request claims,
// so a valid officer credential pair cannot be used to mint an admin token
// by editing the request body's "role" field.
func (h *AuthHandler) loginStaff(w http.ResponseWriter, r *http.Request, req loginRequest) {
	staffMember, err := h.staff.FindByUsername(r.Context(), req.Username)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	if !h.auth.VerifyPassword(staffMember.PasswordHash, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	if string(staffMember.Role) != req.Role {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "role_mismatch"})
		return
	}
	// No account-suspension check here: internal_staff has no status
	// column in the real schema, so a provisioned account is implicitly
	// active until its row is deleted. If suspension is needed later, it
	// requires an actual ALTER TABLE ADD COLUMN status, not just a Go-side
	// check against data that doesn't exist.

	token, err := h.auth.IssueToken(staffMember.ID, string(staffMember.Role))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_generation_failed"})
		return
	}

	initials := "ST"
	if runes := []rune(staffMember.NameEn); len(runes) >= 2 {
		initials = string(runes[:2])
	}

	writeJSON(w, http.StatusOK, loginResponse{
		Token: token,
		User: userProfile{
			ID: staffMember.ID, Role: string(staffMember.Role),
			NameAr: staffMember.NameAr, NameEn: staffMember.NameEn,
			TitleAr: staffMember.TitleAr, TitleEn: staffMember.TitleEn,
			Initials: initials,
			// Status left blank: not applicable to staff accounts, and
			// there's no status column on internal_staff to populate it
			// from. OnboardingCompleted is likewise meaningless here.
		},
	})
}