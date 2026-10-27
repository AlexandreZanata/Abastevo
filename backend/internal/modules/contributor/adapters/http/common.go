package http

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

const maxUserBody = 8 << 10

func readJSON(r *http.Request, dst any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return domain.ErrInvalid
	}
	raw, err := httpapi.ReadBody(r, maxUserBody)
	if err != nil {
		return err
	}
	return httpapi.DecodeJSON(raw, dst)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string) {
	httpapi.WriteError(w, r, status, code, "contributor request refused", nil)
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "contributor.invalid")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, r, http.StatusForbidden, "contributor.forbidden")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "contributor.not-found")
	case errors.Is(err, domain.ErrConflict):
		writeError(w, r, http.StatusConflict, "contributor.conflict")
	case errors.Is(err, domain.ErrQuota):
		writeError(w, r, http.StatusTooManyRequests, "contributor.quota")
	case errors.Is(err, domain.ErrOverrideRequired):
		writeError(w, r, http.StatusConflict, "contributor.override-required")
	case errors.Is(err, domain.ErrClaimState), errors.Is(err, domain.ErrClaimExpired):
		writeError(w, r, http.StatusConflict, "contributor.claim-state")
	case errors.Is(err, admindomain.ErrConflict):
		// Idempotency-Key reused with a different body (ADM-12).
		writeError(w, r, http.StatusConflict, "admin.conflict")
	case errors.Is(err, admindomain.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
	case errors.Is(err, admindomain.ErrUnauthorized), errors.Is(err, admindomain.ErrDenied):
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
	case errors.Is(err, entdomain.ErrIneligible):
		writeError(w, r, http.StatusConflict, "entitlement.account-ineligible")
	case errors.Is(err, entdomain.ErrExists):
		writeError(w, r, http.StatusConflict, "entitlement.exists")
	case errors.Is(err, entdomain.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "entitlement.not-found")
	default:
		writeError(w, r, http.StatusServiceUnavailable, "contributor.unavailable")
	}
}

func principal(ctx context.Context, sessions func(context.Context, string, string) (string, error), family, access string) (string, error) {
	if sessions == nil || !domain.ValidUUID(family) || len(access) == 0 || len(access) > 512 {
		return "", domain.ErrForbidden
	}
	accountID, err := sessions(ctx, family, access)
	if err != nil || !domain.ValidUUID(accountID) {
		return "", domain.ErrForbidden
	}
	return accountID, nil
}

func viewClaim(claim domain.Claim, includeCode bool) map[string]any {
	out := map[string]any{
		"claim_id":        claim.ID,
		"github_login":    claim.GitHubLogin,
		"state":           string(claim.State),
		"proof_status":    string(claim.ProofStatus),
		"override":        claim.Override,
		"created_at":      claim.CreatedAt.UTC(),
		"expires_at":      claim.ExpiresAt.UTC(),
		"decision_reason": claim.DecisionReason,
	}
	if claim.GistID != "" {
		out["gist_id"] = claim.GistID
	}
	if claim.DecidedAt != nil {
		out["decided_at"] = claim.DecidedAt.UTC()
	}
	if includeCode && claim.State == domain.ClaimPending {
		out["code"] = claim.Code
	}
	return out
}

func viewProfile(profile domain.Profile, links []domain.Link) map[string]any {
	return map[string]any{
		"display_name":        profile.DisplayName,
		"links":               domain.PublicLinks(links),
		"visibility":          profile.Visibility,
		"public_consent":      profile.PublicConsent,
		"show_roles_publicly": profile.ShowRolesPublicly,
	}
}

func parseLimit(raw string, fallback, max int) (int, bool) {
	if raw == "" {
		return fallback, true
	}
	n, err := httpapi.ParseLimit(raw)
	if err != nil || n > max {
		return 0, false
	}
	return n, true
}
