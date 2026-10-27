package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

type snapshotCursor struct {
	Dataset string `json:"d"`
	After   string `json:"a"`
	Through int64  `json:"t"`
	Epoch   int64  `json:"e"`
	Consent int64  `json:"c"`
	Expires int64  `json:"exp"`
	Filter  string `json:"fh"`
}

func snapshotFilter(owner, origin, device, dataset string) string {
	return httpapi.FilterHash("cloud-sync-snapshot-v1", origin, owner, device, dataset, "v1")
}

func sealSnapshot(secret []byte, filter, dataset, after string, through, epoch, consent int64, now time.Time) string {
	body, _ := json.Marshal(snapshotCursor{Dataset: dataset, After: after, Through: through, Epoch: epoch, Consent: consent, Expires: now.Add(httpapi.CursorTTL).Unix(), Filter: filter})
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(append(mac.Sum(nil), body...))
}

func openSnapshot(secret []byte, token, filter string, now time.Time) (app.SnapshotQuery, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= sha256.Size {
		return app.SnapshotQuery{}, domain.ErrInvalid
	}
	sum, body := raw[:sha256.Size], raw[sha256.Size:]
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), sum) {
		return app.SnapshotQuery{}, domain.ErrInvalid
	}
	var c snapshotCursor
	if err := json.Unmarshal(body, &c); err != nil {
		return app.SnapshotQuery{}, domain.ErrInvalid
	}
	if c.Filter == "" || c.Filter != filter {
		return app.SnapshotQuery{}, domain.ErrInvalid
	}
	if now.Unix() > c.Expires {
		return app.SnapshotQuery{}, domain.ErrResnapshot
	}
	if c.Through < 0 || c.Epoch < 0 || c.Consent < 0 || (c.After != "" && !domain.ValidID(c.After)) {
		return app.SnapshotQuery{}, domain.ErrInvalid
	}
	return app.SnapshotQuery{Dataset: c.Dataset, After: c.After, Through: c.Through, Epoch: c.Epoch, ConsentRevision: c.Consent}, nil
}

func validDataset(d string) bool { return domain.KnownDataset(d) }

func (h Handler) consent(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Datasets []string `json:"datasets"`
		Enabled  *bool    `json:"enabled"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if dto.Enabled == nil || len(dto.Datasets) < 1 || len(dto.Datasets) > len(domain.Datasets()) {
		fail(w, r, domain.ErrInvalid)
		return
	}
	seen := map[string]bool{}
	for _, d := range dto.Datasets {
		if !validDataset(d) || seen[d] {
			fail(w, r, domain.ErrInvalid)
			return
		}
		seen[d] = true
	}
	p, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	got, err := h.Service.SetConsent(r.Context(), p, dto.Datasets, *dto.Enabled)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}

func (h Handler) importPreview(w http.ResponseWriter, r *http.Request) {
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
	got, err := h.Service.Preview(r.Context(), p, dto.Batch)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}

func (h Handler) syncSnapshot(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		DeviceID string `json:"device_id"`
		Dataset  string `json:"dataset"`
		Limit    *int   `json:"limit,omitempty"`
		Cursor   string `json:"cursor,omitempty"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	limit := 20
	if dto.Limit != nil {
		limit = *dto.Limit
	}
	if limit < 1 || limit > 100 || len(dto.Cursor) > 2048 {
		fail(w, r, domain.ErrInvalid)
		return
	}
	if !domain.ValidID(dto.DeviceID) {
		fail(w, r, domain.ErrInvalid)
		return
	}
	if dto.Dataset != "" && !validDataset(dto.Dataset) {
		fail(w, r, domain.ErrInvalid)
		return
	}
	p, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	p.DeviceID = dto.DeviceID
	if len(h.CursorSecret) < 32 {
		fail(w, r, domain.ErrUnavailable)
		return
	}
	filter := snapshotFilter(p.AccountID, h.Origin, p.DeviceID, dto.Dataset)
	var pos app.SnapshotQuery
	if dto.Cursor == "" {
		if dto.Dataset == "" {
			fail(w, r, domain.ErrInvalid)
			return
		}
		pos = app.SnapshotQuery{Dataset: dto.Dataset}
	} else {
		pos, err = openSnapshot(h.CursorSecret, dto.Cursor, filter, time.Now())
		if err != nil {
			fail(w, r, err)
			return
		}
		if dto.Dataset != "" && dto.Dataset != pos.Dataset {
			fail(w, r, domain.ErrInvalid)
			return
		}
	}
	got, err := h.Service.Snapshot(r.Context(), p, pos, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	after := pos.After
	if len(got.Records) > 0 {
		after = got.Records[len(got.Records)-1].ID
	} else if pos.After == "" {
		after = ""
	}
	next := sealSnapshot(h.CursorSecret, snapshotFilter(p.AccountID, h.Origin, p.DeviceID, pos.Dataset), pos.Dataset, after, got.Through, got.Epoch, got.ConsentRevision, time.Now())
	if got.Records == nil {
		got.Records = []app.Record{}
	}
	write(w, r, struct {
		Records         []app.Record `json:"records"`
		Through         int64        `json:"through"`
		Epoch           int64        `json:"epoch"`
		ConsentRevision int64        `json:"consent_revision"`
		Next            string       `json:"next_cursor"`
	}{got.Records, got.Through, got.Epoch, got.ConsentRevision, next})
}
