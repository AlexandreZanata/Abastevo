package adapters

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/media"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// JPEGInspector is the existing strict still-image policy plus a full bounded
// decode and the EMP-DOC-014 active-content rules. It preserves original
// bytes. Content that is not an admissible JPEG (magic, marker structure,
// pixel caps, decoding) is ErrUnsupported; a JPEG that carries bytes after its
// end-of-image marker or an active-content signature is ErrUnsafe. Other
// formats require their own reviewed inspector; neither a MIME claim nor a
// magic prefix suffices.
type JPEGInspector struct{}

// A progressive four-component image at the 12 MP cap holds about 240 MB of
// coefficients and planes while it decodes, so the process runs one full
// decode at a time instead of multiplying that peak by its transfer slots.
// A wait ends with the transfer's context.
var (
	decodeSlot  = make(chan struct{}, 1)
	decodeImage = func(raw []byte) error {
		_, err := jpeg.Decode(bytes.NewReader(raw))
		return err
	}
)

var _ application.Inspector = JPEGInspector{}

func (JPEGInspector) Inspect(ctx context.Context, raw []byte) (string, error) {
	if ctx.Err() != nil {
		return "", domain.ErrUnavailable
	}
	if len(raw) > 5<<20 {
		return "", domain.ErrTooLarge
	}
	if _, err := media.Inspect(raw); err != nil {
		// The strict structure walk found the end-of-image marker and then
		// more bytes: an appended payload or polyglot, not a damaged photo.
		if errors.Is(err, media.ErrTrailingData) {
			return "", domain.ErrUnsafe
		}
		return "", domain.ErrUnsupported
	}
	if activeContent(raw) {
		return "", domain.ErrUnsafe
	}
	select {
	case decodeSlot <- struct{}{}:
	case <-ctx.Done():
		return "", domain.ErrUnavailable
	}
	err := decodeImage(raw)
	<-decodeSlot
	if err != nil {
		return "", domain.ErrUnsupported
	}
	if ctx.Err() != nil {
		return "", domain.ErrUnavailable
	}
	return "image/jpeg", nil
}
