package http

import (
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
)

func (h Handler) backupManifest(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		DeviceID string `json:"device_id"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if !domain.ValidID(dto.DeviceID) {
		fail(w, r, domain.ErrInvalid)
		return
	}
	p, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	p.DeviceID = dto.DeviceID
	got, err := h.Service.Manifest(r.Context(), p)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}
