package handlers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"yemen-mit-platform/internal/auth"
	"yemen-mit-platform/internal/domain"
	"yemen-mit-platform/internal/repository"
)

type MerchantOnboardingHandler struct {
	merchants repository.MerchantRepository
}

func NewMerchantOnboardingHandler(merchants repository.MerchantRepository) *MerchantOnboardingHandler {
	return &MerchantOnboardingHandler{merchants: merchants}
}

type onboardingResponse struct {
	OnboardingCompleted bool   `json:"onboardingCompleted"`
	Status              string `json:"status"`
	CompanyNameAr       string `json:"companyNameAr"`
	CompanyNameEn       string `json:"companyNameEn"`
}

// Complete handles POST /api/v1/merchant/onboarding (multipart/form-data)
func (h *MerchantOnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "merchant" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "role_mismatch"})
		return
	}

	// 1. استقبال الـ Multipart Form (حد أقصى 20MB)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_form_data"})
		return
	}

	taxID := r.FormValue("taxId")
	crn := r.FormValue("commercialRegisterNo")
	signatoryName := r.FormValue("authorizedSignatoryName")
	signatoryID := r.FormValue("authorizedSignatoryId")
	phone := r.FormValue("phone")
	city := r.FormValue("city")
	addressLine := r.FormValue("addressLine")
	tradeNatureStr := r.FormValue("tradeNature")
	goodsCategory := r.FormValue("primaryGoodsCategory")
	countryOrigin := r.FormValue("defaultCountryOfOrigin")
	portEntry := r.FormValue("defaultPortOfEntry")

	
	if taxID == "" || crn == "" || signatoryName == "" || signatoryID == "" ||
		addressLine == "" || city == "" || phone == "" || goodsCategory == "" ||
		countryOrigin == "" || portEntry == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_fields"})
		return
	}

	var gpsLat, gpsLng float64
	if latStr := r.FormValue("gpsLat"); latStr != "" {
		gpsLat, _ = strconv.ParseFloat(latStr, 64)
	}
	if lngStr := r.FormValue("gpsLng"); lngStr != "" {
		gpsLng, _ = strconv.ParseFloat(lngStr, 64)
	}

	tradeNature := domain.MerchantTradeNature(tradeNatureStr)
	if !tradeNature.IsValid() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_trade_nature"})
		return
	}

	// 2. قراءة الملفات وتحميلها
	files := r.MultipartForm.File["documents"]
	var docs []domain.MerchantDocument

	if len(files) > 0 {
		uploadDir := filepath.Join("uploads", "merchants", claims.MerchantID)
		_ = os.MkdirAll(uploadDir, 0755)

		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				continue
			}

			filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(fileHeader.Filename))
			filePath := filepath.Join(uploadDir, filename)

			dst, err := os.Create(filePath)
			if err != nil {
				file.Close()
				continue
			}

			_, _ = io.Copy(dst, file)
			file.Close()
			dst.Close()

			docs = append(docs, domain.MerchantDocument{
				Type:       "license_doc",
				URL:        fmt.Sprintf("/uploads/merchants/%s/%s", claims.MerchantID, filename),
				UploadedAt: time.Now(),
			})
		}
	}

	// 3. بناء الـ Input بالطابق التام لـ domain.MerchantOnboardingInput الأصلي
	input := domain.MerchantOnboardingInput{
		TaxID:                   taxID,
		AuthorizedSignatoryName: signatoryName,
		AuthorizedSignatoryID:   signatoryID,
		AddressLine:             addressLine,
		City:                    city,
		GPSLat:                  gpsLat,
		GPSLng:                  gpsLng,
		TradeNature:             tradeNature,
		PrimaryGoodsCategory:    goodsCategory,
		DefaultCountryOfOrigin:  countryOrigin,
		DefaultPortOfEntry:      portEntry,
		DefaultExporterName:     r.FormValue("defaultExporterName"),
		CommercialRegisterNo:    crn,
		Phone:                   phone,
		LegalDocuments:          docs,
	}

	updated, err := h.merchants.CompleteOnboarding(r.Context(), claims.MerchantID, input)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrDuplicateCRN):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "crn_already_registered"})
		case errors.Is(err, repository.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "merchant_not_found"})
		default:
			log.Printf("merchant onboarding: CompleteOnboarding failed for merchant %s: %v", claims.MerchantID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "onboarding_failed"})
		}
		return
	}

	writeJSON(w, http.StatusOK, onboardingResponse{
		OnboardingCompleted: updated.OnboardingCompleted,
		Status:              string(updated.Status),
		CompanyNameAr:       updated.CompanyNameAr,
		CompanyNameEn:       updated.CompanyNameEn,
	})
}

// Me handles GET /api/v1/merchant/me
func (h *MerchantOnboardingHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "merchant" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "role_mismatch"})
		return
	}

	merchant, err := h.merchants.FindByID(r.Context(), claims.MerchantID)
	if err != nil {
		log.Printf("merchant self-status: FindByID failed for merchant %s: %v", claims.MerchantID, err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "merchant_not_found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"onboardingCompleted": merchant.OnboardingCompleted,
		"status":              string(merchant.Status),
		"walletBalance":       merchant.WalletBalance,
	})
}