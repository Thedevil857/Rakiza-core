package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"yemen-mit-platform/internal/auth"
	"yemen-mit-platform/internal/domain"
	"yemen-mit-platform/internal/repository"
)

type CheckpointHandler struct {
	clearances     repository.PermitClearanceRepository
	qrPublicKeyHex string
}

func NewCheckpointHandler(clearances repository.PermitClearanceRepository, qrPublicKeyHex string) *CheckpointHandler {
	return &CheckpointHandler{clearances: clearances, qrPublicKeyHex: qrPublicKeyHex}
}

// PublicKey handles GET /api/checkpoint/public-key. The officer's app calls
// this exactly once -- while it still has signal, typically right after
// login -- and caches the result in localStorage. From then on, verifying
// a scanned QR code's signature happens entirely in the browser via the
// Web Crypto API against this cached key; this endpoint is never called
// again unless the cache is cleared.
func (h *CheckpointHandler) PublicKey(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "officer" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "officer_role_required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"publicKeyHex": h.qrPublicKeyHex})
}

type clearanceSubmission struct {
	PermitID  string `json:"permitId"`
	Decision  string `json:"decision"`
	DecidedAt string `json:"decidedAt"` // RFC3339, set on the officer's phone at decision time
}

type submitClearancesRequest struct {
	Clearances []clearanceSubmission `json:"clearances"`
}

// SubmitClearances handles POST /api/checkpoint/clearances. The officer's
// phone calls this once it regains connectivity, submitting every
// pass/reject decision made while offline in a single batch. Each entry is
// processed independently -- one bad entry in a batch does not block the
// rest from syncing -- and the response reports per-entry status so the
// client knows exactly which decisions are safe to drop from its local
// queue and which still need to be retried.
func (h *CheckpointHandler) SubmitClearances(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "officer" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "officer_role_required"})
		return
	}

	var req submitClearancesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	if len(req.Clearances) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_batch"})
		return
	}

	results := make([]map[string]string, 0, len(req.Clearances))
	for _, c := range req.Clearances {
		decision := domain.PermitClearanceDecision(c.Decision)
		if c.PermitID == "" || !decision.IsValid() {
			results = append(results, map[string]string{"permitId": c.PermitID, "status": "invalid"})
			continue
		}

		decidedAt, err := time.Parse(time.RFC3339, c.DecidedAt)
		if err != nil {
			// Falls back to server-receipt time rather than rejecting the
			// whole entry -- a malformed timestamp from an offline device
			// shouldn't cost the officer their decision, just its exact
			// original timing.
			decidedAt = time.Now().UTC()
		}

		_, err = h.clearances.Create(r.Context(), domain.PermitClearance{
			PermitID: c.PermitID, OfficerID: claims.MerchantID, Decision: decision, DecidedAt: decidedAt,
		})
		if err != nil {
			log.Printf("checkpoint: failed to store clearance for permit %s: %v", c.PermitID, err)
			results = append(results, map[string]string{"permitId": c.PermitID, "status": "failed"})
			continue
		}
		results = append(results, map[string]string{"permitId": c.PermitID, "status": "synced"})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}