package http

import (
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
)

func (h Handler) snapshotsList(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		DeviceID string `json:"device_id"`
		Limit    *int   `json:"limit,omitempty"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	limit := 20
	if dto.Limit != nil {
		limit = *dto.Limit
	}
	if limit < 1 || limit > 100 {
		fail(w, r, domain.ErrInvalid)
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
	rows, err := h.Service.ListSnapshots(r.Context(), p, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	type view struct {
		SnapshotID      string `json:"snapshot_id"`
		Revision        int64  `json:"revision"`
		Epoch           int64  `json:"epoch"`
		ConsentRevision int64  `json:"consent_revision"`
		ByteSize        int64  `json:"byte_size"`
		Checksum        string `json:"checksum"`
		Status          string `json:"status"`
	}
	views := make([]view, 0, len(rows))
	for _, row := range rows {
		views = append(views, view{row.SnapshotID, row.Revision, row.Epoch, row.ConsentRevision, row.ByteSize, row.Checksum, row.Status})
	}
	write(w, r, struct {
		Snapshots []view `json:"snapshots"`
	}{views})
}

func (h Handler) restorePreview(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		DeviceID   string `json:"device_id"`
		SnapshotID string `json:"snapshot_id"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if !domain.ValidID(dto.DeviceID) || !domain.ValidID(dto.SnapshotID) {
		fail(w, r, domain.ErrInvalid)
		return
	}
	p, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	p.DeviceID = dto.DeviceID
	got, err := h.Service.PreviewRestore(r.Context(), p, dto.SnapshotID)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}
