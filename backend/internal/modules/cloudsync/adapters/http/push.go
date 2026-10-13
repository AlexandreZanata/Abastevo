package http

import (
	"encoding/json"
	"errors"
	"net/http"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
)

func (h Handler) syncPush(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		DeviceID string `json:"device_id"`
		app.Batch
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
	got, err := h.Service.Apply(r.Context(), p, dto.Batch)
	if err != nil {
		var conflict *app.ConflictError
		if errors.As(err, &conflict) {
			body, _ := json.Marshal(struct {
				Epoch     int64          `json:"epoch"`
				Revision  int64          `json:"revision"`
				Conflicts []app.Conflict `json:"conflicts"`
			}{conflict.Epoch, conflict.Revision, []app.Conflict{conflict.Conflict}})
			w.Header().Del("ETag")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write(body)
			return
		}
		fail(w, r, err)
		return
	}
	write(w, r, got)
}
