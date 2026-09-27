package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"yemen-mit-platform/internal/auth"
	"yemen-mit-platform/internal/repository"
)

type MerchantReviewHandler struct {
	merchants repository.MerchantRepository
}

func NewMerchantReviewHandler(merchants repository.MerchantRepository) *MerchantReviewHandler {
	return &MerchantReviewHandler{merchants: merchants}
}

type legalDocumentView struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type pendingMerchantView struct {
	ID                      string              `json:"id"`
	CompanyNameAr           string              `json:"companyNameAr"`
	CompanyNameEn           string              `json:"companyNameEn"`
	CommercialRegisterNo    string              `json:"commercialRegisterNo"`
	Phone                   string              `json:"phone"`
	TaxID                   string              `json:"taxId"`
	AuthorizedSignatoryName string              `json:"authorizedSignatoryName"`
	AuthorizedSignatoryID   string              `json:"authorizedSignatoryId"`
	AddressLine             string              `json:"addressLine"`
	City                    string              `json:"city"`
	TradeNature             string              `json:"tradeNature"`
	PrimaryGoodsCategory    string              `json:"primaryGoodsCategory"`
	DefaultCountryOfOrigin  string              `json:"defaultCountryOfOrigin"`
	DefaultPortOfEntry      string              `json:"defaultPortOfEntry"`
	DefaultExporterName     string              `json:"defaultExporterName"`
	LegalDocuments          []legalDocumentView `json:"legalDocuments"`
	SubmittedAt             string              `json:"submittedAt"`
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ListPending handles GET /api/admin/merchants/pending -- the Minister's
// review queue: every merchant who has finished the onboarding wizard and
// is waiting for their account to be activated.
func (h *MerchantReviewHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "role_mismatch"})
		return
	}

	merchants, err := h.merchants.ListPendingReview(r.Context())
	if err != nil {
		log.Printf("merchant review: ListPendingReview failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_pending_failed"})
		return
	}

	view := make([]pendingMerchantView, 0, len(merchants))
	for _, m := range merchants {
		docs := make([]legalDocumentView, 0, len(m.LegalDocuments))
		for _, d := range m.LegalDocuments {
			docs = append(docs, legalDocumentView{Type: d.Type, URL: d.URL})
		}
		tradeNature := ""
		if m.TradeNature != nil {
			tradeNature = string(*m.TradeNature)
		}
		view = append(view, pendingMerchantView{
			ID: m.ID, CompanyNameAr: m.CompanyNameAr, CompanyNameEn: m.CompanyNameEn,
			CommercialRegisterNo: m.CommercialRegisterNo, Phone: m.Phone,
			TaxID:                   strOrEmpty(m.TaxID),
			AuthorizedSignatoryName: strOrEmpty(m.AuthorizedSignatoryName),
			AuthorizedSignatoryID:   strOrEmpty(m.AuthorizedSignatoryID),
			AddressLine:             strOrEmpty(m.AddressLine),
			City:                    strOrEmpty(m.City),
			TradeNature:             tradeNature,
			PrimaryGoodsCategory:    strOrEmpty(m.PrimaryGoodsCategory),
			DefaultCountryOfOrigin:  strOrEmpty(m.DefaultCountryOfOrigin),
			DefaultPortOfEntry:      strOrEmpty(m.DefaultPortOfEntry),
			DefaultExporterName:     strOrEmpty(m.DefaultExporterName),
			LegalDocuments:          docs,
			SubmittedAt:             m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	writeJSON(w, http.StatusOK, view)
}

// Approve handles PATCH /api/admin/merchants/{id}/approve -- the Minister
// accepting a reviewed submission. Transitions the merchant from Pending to
// Active, unlocking full platform access (including budget-related
// services) on their next status check.
func (h *MerchantReviewHandler) Approve(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "role_mismatch"})
		return
	}

	merchantID := chi.URLParam(r, "id")
	if merchantID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_merchant_id"})
		return
	}

	updated, err := h.merchants.ApproveMerchant(r.Context(), merchantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "merchant_not_pending_or_not_found"})
			return
		}
		log.Printf("merchant review: ApproveMerchant failed for %s: %v", merchantID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approve_failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":     updated.ID,
		"status": string(updated.Status),
	})
}