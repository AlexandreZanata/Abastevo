package adapters

import (
	"net/http"
	"strconv"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// WriteContent delivers only the audited Service.Open result. Owning controllers
// authenticate their POST/body token and use the existing private error envelope.
// No original filename, storage URL or conditional-cache response is emitted.
func WriteContent(w http.ResponseWriter, content application.Content) error {
	limit, err := domain.ByteLimit(content.MIME)
	if err != nil || len(content.Bytes) == 0 || int64(len(content.Bytes)) > limit {
		return domain.ErrInvalid
	}
	extension := map[string]string{"image/jpeg": "jpg", "image/png": "png", "application/pdf": "pdf", "application/xml": "xml"}[content.MIME]
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("ETag")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", content.MIME)
	w.Header().Set("Content-Disposition", `attachment; filename="document.`+extension+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(content.Bytes)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(content.Bytes); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
