package adapters

import (
	"context"
	"errors"

	evidencemedia "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/media"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

var ErrTrainingCollectionUnavailable = errors.New("training collection: content minimization not qualified")

// TrainingSourceUnavailable is the production collection gate. There is no
// configuration switch to bypass the unqualified minimizer. Cleanup remains
// independent and must continue for any historical inventoried bytes.
func TrainingSourceUnavailable(context.Context, string) ([]byte, error) {
	return nil, ErrTrainingCollectionUnavailable
}

// SanitizeImage validates only the bounded JPEG container. Removing EXIF does
// not exclude identifying pixels or text. Until a price-region minimizer is
// qualified, even valid input is UNCERTAIN and no bytes leave this adapter.
// Do not decode or re-encode an image for an unavailable capability.
func SanitizeImage(ctx context.Context, source []byte) ([]byte, domain.SanitizeVerdict, error) {
	if err := ctx.Err(); err != nil {
		return nil, domain.VerdictFail, err
	}
	if len(source) > 3<<20 {
		return nil, domain.VerdictFail, evidencemedia.ErrTooLarge
	}
	if _, err := evidencemedia.Inspect(source); err != nil {
		return nil, domain.VerdictFail, err
	}
	return nil, domain.VerdictUncertain, nil
}
