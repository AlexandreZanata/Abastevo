package adapters

import (
	"context"

	evidencemedia "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/media"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// SanitizeImage adapts the evidence media pipeline onto the promotion
// Sanitizer port (CS10B, AT03): bounded JPEG structure validation
// plus EXIF-free re-encode. Success minimizes (PASS); every pipeline
// refusal maps to FAIL — fail-closed with no fallback and no
// original-photo retention. UNCERTAIN stays reserved for a future
// content classifier; nothing here invents one. The media package is
// stateless and stdlib-only, so sharing it here crosses no module
// ledger or query ownership.
func SanitizeImage(_ context.Context, source []byte) ([]byte, domain.SanitizeVerdict, error) {
	minimized, err := evidencemedia.Sanitize(source)
	if err != nil {
		return nil, domain.VerdictFail, err
	}
	return minimized, domain.VerdictPass, nil
}
