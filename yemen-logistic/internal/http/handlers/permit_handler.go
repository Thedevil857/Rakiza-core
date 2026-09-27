package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"

	"yemen-mit-platform/internal/auth"
	"yemen-mit-platform/internal/checkpoint"
	"yemen-mit-platform/internal/domain"
	repository "yemen-mit-platform/internal/repository"
)

type PermitHandler struct {
	permits         repository.PermitRepository
	merchants       repository.MerchantRepository
	qrPrivateKeyHex string
	seq             atomic.Int64
}

func NewPermitHandler(permits repository.PermitRepository, merchants repository.MerchantRepository, qrPrivateKeyHex string) *PermitHandler {
	h := &PermitHandler{permits: permits, merchants: merchants, qrPrivateKeyHex: qrPrivateKeyHex}
	h.seq.Store(1000)
	return h
}

type createPermitRequest struct {
	Type           string  `json:"type"`
	CategoryID     string  `json:"category_id"`
	Weight         float64 `json:"weight"`
	Value          float64 `json:"value"`
	TruckNumber    string  `json:"truck_number"`
	ProductionDate string  `json:"production_date"` // YYYY-MM-DD
	ExpiryDate     string  `json:"expiry_date"`     // YYYY-MM-DD
}

// Create authenticates the merchant via the JWT claims injected by
// auth.Middleware, computes the dynamic category duty, atomically deducts it
// from the merchant's budget, and only then persists the permit with dates & truck details.
func (h *PermitHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "merchant" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "merchant_role_required"})
		return
	}

	var req createPermitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_payload"})
		return
	}
	if req.Type == "" || req.Weight <= 0 || req.Value <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cargo_details"})
		return
	}


	var prodDate, expDate *time.Time
	if req.ProductionDate != "" {
		if t, err := time.Parse("2006-01-02", req.ProductionDate); err == nil {
			prodDate = &t
		}
	}
	if req.ExpiryDate != "" {
		if t, err := time.Parse("2006-01-02", req.ExpiryDate); err == nil {
			expDate = &t
		}
	}


	duty := domain.CalculateDuty(req.Value, req.CategoryID)

	if _, err := h.merchants.DeductBudget(r.Context(), claims.MerchantID, duty); err != nil {
		if errors.Is(err, repository.ErrInsufficientBudget) {
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "insufficient_budget"})
			return
		}
		log.Printf("permit create: DeductBudget failed for merchant %s: %v", claims.MerchantID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "budget_check_failed"})
		return
	}

	id := fmt.Sprintf("YEM-%d-%d",time.Now().Unix(), h.seq.Add(1))
	permit := domain.Permit{
		ID:             id,
		MerchantID:     claims.MerchantID,
		Type:           req.Type,
		CategoryID:     req.CategoryID,
		Weight:         req.Weight,
		Value:          req.Value,
		Duty:           duty,
		TruckNumber:    req.TruckNumber,
		ProductionDate: prodDate,
		ExpiryDate:     expDate,
		Status:         domain.PermitStatusCleared,
	}

	created, err := h.permits.Create(r.Context(), permit)
	if err != nil {
		log.Printf("permit create: h.permits.Create failed for merchant %s: %v", claims.MerchantID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "permit_creation_failed"})
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// GetQR generates a real, cryptographically signed QR code for a permit --
// replacing the previous placeholder, which encoded a plain descriptive
// string with no signature at all. The signed token embeds the merchant's
// display name and full cargo details directly (see PermitQRClaims), so an
// officer's phone can show everything relevant with zero network access;
// it never needs to look this permit up against a database it can't reach.
func (h *PermitHandler) GetQR(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_permit_id"})
		return
	}

	permit, err := h.permits.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "permit_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup_failed"})
		return
	}

	merchant, err := h.merchants.FindByID(r.Context(), permit.MerchantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "merchant_lookup_failed"})
		return
	}

	signedToken, err := checkpoint.GenerateSecurePermitQRToken(permit, merchant, h.qrPrivateKeyHex)
	if err != nil {
		log.Printf("permit QR: signing failed for permit %s: %v", permit.ID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "qr_signing_failed"})
		return
	}

	// Low error correction (not Medium) and a larger output size: an RS256-
	// signed JWT is much longer than a typical QR payload, which already
	// forces a denser QR (more, smaller modules) even before considering
	// that this is being scanned screen-to-camera between two phones, not
	// from a printed code. Real security here comes from the signature
	// itself, not from QR error-correction redundancy, so it's safe to
	// trade some of that redundancy for a QR code that actually decodes
	// reliably at a normal scanning distance.
	pngBytes, err := qrcode.Encode(signedToken, qrcode.Low, 480)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "qr_generation_failed"})
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(pngBytes)
}