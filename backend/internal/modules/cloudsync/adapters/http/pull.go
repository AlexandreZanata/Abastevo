package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"net/http"
	"time"
)

type pullCursor struct {
	Epoch   int64  `json:"e"`
	Consent int64  `json:"c"`
	After   int64  `json:"a"`
	Through int64  `json:"t"`
	Expires int64  `json:"exp"`
	Filter  string `json:"fh"`
}

func pullFilter(owner, origin, device string) string {
	return httpapi.FilterHash("cloud-sync-pull-v1", origin, owner, device, "v1")
}

func sealPull(secret []byte, filter string, epoch, consent, after, through int64, now time.Time) string {
	body, _ := json.Marshal(pullCursor{Epoch: epoch, Consent: consent, After: after, Through: through, Expires: now.Add(httpapi.CursorTTL).Unix(), Filter: filter})
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(append(mac.Sum(nil), body...))
}

func openPull(secret []byte, token, filter string, now time.Time) (app.Position, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= sha256.Size {
		return app.Position{}, domain.ErrInvalid
	}
	sum, body := raw[:sha256.Size], raw[sha256.Size:]
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), sum) {
		return app.Position{}, domain.ErrInvalid
	}
	var c pullCursor
	if err := json.Unmarshal(body, &c); err != nil {
		return app.Position{}, domain.ErrInvalid
	}
	if c.Filter == "" || c.Filter != filter {
		return app.Position{}, domain.ErrInvalid
	}
	if now.Unix() > c.Expires {
		return app.Position{}, domain.ErrResnapshot
	}
	if c.Epoch < 0 || c.Consent < 0 || c.After < 0 || c.Through < 0 {
		return app.Position{}, domain.ErrInvalid
	}
	if c.Through != 0 && c.After > c.Through {
		return app.Position{}, domain.ErrInvalid
	}
	return app.Position{Version: 1, Epoch: c.Epoch, ConsentRevision: c.Consent, After: c.After, Through: c.Through}, nil
}

func (h Handler) syncPull(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		DeviceID string `json:"device_id"`
		Cursor   string `json:"cursor,omitempty"`
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
	if limit < 1 || limit > 100 || len(dto.Cursor) > 2048 {
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
	if len(h.CursorSecret) < 32 {
		fail(w, r, domain.ErrUnavailable)
		return
	}
	filter := pullFilter(p.AccountID, h.Origin, p.DeviceID)
	var pos app.Position
	if dto.Cursor == "" {
		pos = app.Position{Version: 1}
	} else {
		pos, err = openPull(h.CursorSecret, dto.Cursor, filter, time.Now())
		if err != nil {
			fail(w, r, err)
			return
		}
	}
	feed, err := h.Service.Pull(r.Context(), p, pos, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	var nextAfter int64
	if len(feed.Changes) > 0 {
		nextAfter = feed.Changes[len(feed.Changes)-1].Sequence
	} else {
		nextAfter = feed.Through
	}
	next := sealPull(h.CursorSecret, filter, feed.Epoch, feed.ConsentRevision, nextAfter, feed.Through, time.Now())
	if feed.Changes == nil {
		feed.Changes = []app.Change{}
	}
	write(w, r, struct {
		Changes         []app.Change `json:"changes"`
		Through         int64        `json:"through"`
		Epoch           int64        `json:"epoch"`
		ConsentRevision int64        `json:"consent_revision"`
		Next            string       `json:"next_cursor"`
	}{feed.Changes, feed.Through, feed.Epoch, feed.ConsentRevision, next})
}
