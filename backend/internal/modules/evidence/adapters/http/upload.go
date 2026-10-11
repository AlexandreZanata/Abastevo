package http

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// UploadGateway enforces bytes before any storage PUT. Its bearer capability is
// scoped to one reservation and five minutes; no storage credential leaves API.
type UploadGateway struct {
	secret   []byte
	origin   string
	now      func() time.Time
	slots    chan struct{}
	NewNonce func() (string, error)
	Claim    func(context.Context, string, string, time.Time) (domain.Session, bool, error)
	Finish   func(context.Context, string, string, bool, int64) error
	Put      func(context.Context, string, string, []byte) error
}

func NewUploadGateway(secret []byte, origin string, now func() time.Time) *UploadGateway {
	return &UploadGateway{secret: append([]byte{}, secret...), origin: strings.TrimRight(origin, "/"), now: now, slots: make(chan struct{}, 4), NewNonce: func() (string, error) {
		var b [16]byte
		_, err := rand.Read(b[:])
		return hex.EncodeToString(b[:]), err
	}}
}

func (g *UploadGateway) mac(id, expiry string) []byte {
	h := hmac.New(sha256.New, g.secret)
	_, _ = io.WriteString(h, "evidence-upload-v1\n"+id+"\n"+expiry)
	return h.Sum(nil)
}

func (g *UploadGateway) Authorize(_ context.Context, s domain.Session) (string, map[string]string, time.Time, error) {
	now := g.now()
	expiry := minTime(now.Add(5*time.Minute), s.ExpiresAt).Truncate(time.Second)
	u, err := url.Parse(g.origin)
	if err != nil || u.Host == "" || u.User != nil || len(g.secret) < 32 || !expiry.After(now) || s.Status != domain.StateIssued {
		return "", nil, time.Time{}, domain.ErrBadTransition
	}
	u.Path = "/v1/uploads/" + s.ID + "/bytes"
	stamp := strconv.FormatInt(expiry.Unix(), 10)
	q := url.Values{"expires": {stamp}, "token": {hex.EncodeToString(g.mac(s.ID, stamp))}}
	u.RawQuery = q.Encode()
	return u.String(), map[string]string{"Content-Type": s.MIME}, expiry, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (g *UploadGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	refuse := func(code int, kind string) {
		httpapi.WriteError(w, r, code, "evidence."+kind, "photo transfer refused", nil)
	}
	if r.Method != http.MethodPut {
		refuse(405, "method")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "uploads" || parts[3] != "bytes" || len(parts[2]) > 64 {
		refuse(403, "upload-capability")
		return
	}
	id := parts[2]
	stamp := r.URL.Query().Get("expires")
	expiry, err := strconv.ParseInt(stamp, 10, 64)
	token, decodeErr := hex.DecodeString(r.URL.Query().Get("token"))
	now := g.now()
	if err != nil || decodeErr != nil || len(g.secret) < 32 || len(token) != sha256.Size || !hmac.Equal(token, g.mac(id, stamp)) || expiry <= now.Unix() || expiry > now.Add(5*time.Minute).Unix() {
		refuse(403, "upload-capability")
		return
	}
	if g.Claim == nil || g.Finish == nil || g.Put == nil {
		refuse(503, "storage-unavailable")
		return
	}
	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	default:
		w.Header().Set("Retry-After", "2")
		refuse(503, "overloaded")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	nonce, err := g.NewNonce()
	if err != nil {
		refuse(503, "storage-unavailable")
		return
	}
	session, replayed, err := g.Claim(ctx, id, nonce, now)
	if err != nil {
		if errors.Is(err, domain.ErrQuotaDenied) {
			w.Header().Set("Retry-After", "30")
			refuse(429, "upload-budget")
		} else {
			refuse(503, "upload-unavailable")
		}
		return
	}
	if session.Status != domain.StateIssued || !now.Before(session.ExpiresAt) || session.DeclaredBytes < 1 || session.DeclaredBytes > domain.MaxUploadBytes {
		refuse(403, "upload-expired")
		return
	}
	completed := replayed
	defer func() {
		if !completed {
			cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer done()
			_ = g.Finish(cleanup, id, nonce, false, 0)
		}
	}()
	if r.Header.Get("Content-Type") != session.MIME {
		refuse(415, "upload-type")
		return
	}
	if r.ContentLength > session.DeclaredBytes {
		refuse(413, "upload-size")
		return
	}
	if replayed {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(15 * time.Second))
	raw, err := io.ReadAll(io.LimitReader(r.Body, session.DeclaredBytes+1))
	if int64(len(raw)) > session.DeclaredBytes {
		refuse(413, "upload-size")
		return
	}
	if err != nil || int64(len(raw)) != session.DeclaredBytes {
		refuse(422, "upload-size")
		return
	}
	digest := sha256.Sum256(raw)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), session.ClaimedSHA256) {
		refuse(422, "upload-hash")
		return
	}
	if err = g.Put(ctx, session.QuarantineKey, session.MIME, raw); err != nil {
		refuse(503, "storage-unavailable")
		return
	}
	if err = g.Finish(ctx, id, nonce, true, int64(len(raw))); err != nil {
		refuse(503, "upload-unavailable")
		return
	}
	completed = true
	w.WriteHeader(http.StatusNoContent)
}
