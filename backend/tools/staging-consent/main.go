// Command staging-consent verifies the owned staging deployment with two
// ephemeral synthetic identities. It never reads real accounts or media.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/profile"
	legal "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/legalterms"
)

const origin = "https://teste.abastevo.com.br"
const consentPath = "/v1/privacy/training-consent"
const maxResponse = 64 << 10

type probe struct {
	client   *http.Client
	requests int
	checks   []string
}
type owner struct {
	key               *ecdsa.PrivateKey
	fingerprint, x, y string
}
type challenge struct {
	ID    string `json:"challenge_id"`
	Nonce string `json:"nonce"`
}
type result struct {
	status int
	header http.Header
	body   []byte
}

func main() {
	allow := flag.Bool("allow-staging-writes", false, "explicitly enroll two synthetic owners and grant/revoke their consent")
	station := flag.String("community-station", "", "owned synthetic station UUID for community integration verification")
	flag.Parse()
	if !*allow || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "explicit --allow-staging-writes required; only the owned staging origin is supported")
		os.Exit(2)
	}
	p := probe{client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var err error
	if *station != "" {
		err = p.runCommunity(ctx, *station)
	} else {
		err = p.run(ctx)
	}
	report := map[string]any{"origin": origin, "checked_at": time.Now().UTC().Format(time.RFC3339), "checks": p.checks, "requests": p.requests, "passed": err == nil, "collection_available": nil}
	if len(p.checks) > 0 && p.checks[0] == "exact-published-terms-off" {
		report["collection_available"] = false
	}
	if err != nil {
		report["failure"] = err.Error()
	}
	json.NewEncoder(os.Stdout).Encode(report)
	if err != nil {
		os.Exit(1)
	}
}

func (p *probe) send(req *http.Request) (result, error) {
	if p.requests >= 64 {
		return result{}, errors.New("request budget exhausted")
	}
	p.requests++
	res, err := p.client.Do(req)
	if err != nil {
		return result{}, errors.New("HTTPS request failed")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(b) > maxResponse {
		return result{}, errors.New("response unavailable or too large")
	}
	return result{res.StatusCode, res.Header, b}, nil
}
func request(ctx context.Context, method, path string, b []byte) *http.Request {
	r, err := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(b))
	if err != nil {
		panic("invalid fixed probe target")
	}
	r.Header.Set("Accept", "application/json")
	if len(b) > 0 {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}
func payload(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic("invalid probe payload")
	}
	return b
}
func require(r result, status int) error {
	if r.status != status {
		return fmt.Errorf("unexpected HTTP status %d, expected %d", r.status, status)
	}
	if r.header.Get("Cache-Control") != "no-store" {
		return errors.New("private response missing no-store")
	}
	return nil
}
func (p *probe) check(name string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	p.checks = append(p.checks, name)
	return nil
}
func newOwner() (*owner, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	pub, err := k.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	enc := base64.RawURLEncoding.EncodeToString
	x, y := enc(pub[1:33]), enc(pub[33:])
	fp, err := profile.Thumbprint(x, y)
	return &owner{k, fp, x, y}, err
}
func sign(o *owner, lines []string) (string, error) {
	d := sha512.Sum512([]byte(strings.Join(lines, "\n")))
	r, s, err := ecdsa.Sign(rand.Reader, o.key, d[:])
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)), nil
}
func (p *probe) challenge(ctx context.Context, o *owner, purpose string) (challenge, error) {
	r, err := p.send(request(ctx, "POST", "/v1/identity/challenges", payload(map[string]string{"fingerprint": o.fingerprint, "purpose": purpose})))
	if err != nil {
		return challenge{}, err
	}
	if err = require(r, 201); err != nil {
		return challenge{}, err
	}
	var ch challenge
	err = json.Unmarshal(r.body, &ch)
	if err == nil && (ch.ID == "" || ch.Nonce == "") {
		err = errors.New("missing challenge")
	}
	return ch, err
}
func lines(req *http.Request, o *owner, ch challenge, b []byte) ([]string, string, string) {
	now := time.Now()
	created, expires := strconv.FormatInt(now.Add(-time.Minute).Unix(), 10), strconv.FormatInt(now.Add(4*time.Minute).Unix(), 10)
	u, _ := url.Parse(origin)
	return auth.BaseLines(req, u.Host, created, expires, o.fingerprint, ch.Nonce, b, len(b) > 0), created, expires
}
func (p *probe) enroll(ctx context.Context) (*owner, error) {
	o, err := newOwner()
	if err != nil {
		return nil, err
	}
	ch, err := p.challenge(ctx, o, "REGISTER")
	if err != nil {
		return nil, err
	}
	req := request(ctx, "POST", "/v1/contributors", nil)
	ls, _, _ := lines(req, o, ch, nil)
	sig, err := sign(o, ls)
	if err != nil {
		return nil, err
	}
	b := payload(map[string]any{"public_jwk": map[string]string{"kty": "EC", "crv": "P-256", "x": o.x, "y": o.y}, "challenge_id": ch.ID, "proof": map[string]any{"lines": ls, "signature": sig}})
	res, err := p.send(request(ctx, "POST", "/v1/contributors", b))
	if err == nil {
		err = require(res, 201)
	}
	return o, err
}
func (p *probe) signed(ctx context.Context, o *owner, method, path string, b []byte, forge bool) (result, error) {
	ch, err := p.challenge(ctx, o, "SIGN")
	if err != nil {
		return result{}, err
	}
	r := request(ctx, method, path, b)
	ls, c, e := lines(r, o, ch, b)
	sig, err := sign(o, ls)
	if err != nil {
		return result{}, err
	}
	if forge {
		sig = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	}
	if method == "POST" {
		var body map[string]any
		if json.Unmarshal(b, &body) == nil {
			for _, field := range []string{"client_submission_id", "client_capture_id"} {
				if value, ok := body[field].(string); ok {
					r.Header.Set("Idempotency-Key", value)
					break
				}
			}
		}
	}
	r.Header.Set(auth.HeaderSignature, sig)
	r.Header.Set(auth.HeaderCreated, c)
	r.Header.Set(auth.HeaderExpires, e)
	r.Header.Set(auth.HeaderKeyID, o.fingerprint)
	r.Header.Set(auth.HeaderNonce, ch.Nonce)
	return p.send(r)
}
func decode(r result, status int, state string, revision uint64) (map[string]any, error) {
	if err := require(r, status); err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(r.body, &v); err != nil {
		return nil, errors.New("invalid JSON")
	}
	if state != "" && (v["state"] != state || v["revision"] != float64(revision)) {
		return nil, errors.New("unexpected decision state/revision")
	}
	return v, nil
}
func (p *probe) decision(ctx context.Context, o *owner, path string, b []byte, status int, state string, rev uint64) (map[string]any, error) {
	r, err := p.signed(ctx, o, "POST", consentPath+path, b, false)
	if err != nil {
		return nil, err
	}
	return decode(r, status, state, rev)
}
func (p *probe) read(ctx context.Context, o *owner) (map[string]any, error) {
	r, err := p.signed(ctx, o, "GET", consentPath, nil, false)
	if err != nil {
		return nil, err
	}
	return decode(r, 200, "", 0)
}
func (p *probe) run(ctx context.Context) (runErr error) {
	r, err := p.send(request(ctx, "GET", "/v1/privacy/training-terms", nil))
	if err != nil {
		return err
	}
	if err = require(r, 200); err != nil {
		return err
	}
	var terms struct {
		Version, Digest, Text, Locale string
		CollectionAvailable           bool `json:"collection_available"`
	}
	if err = json.Unmarshal(r.body, &terms); err != nil {
		return errors.New("invalid public terms")
	}
	if terms.Version != legal.Version || terms.Digest != legal.Digest || terms.Text != legal.Text || terms.Locale != "pt-BR" || terms.CollectionAvailable {
		return errors.New("public terms mismatch")
	}
	p.checks = append(p.checks, "exact-published-terms-off")
	for _, method := range []string{"GET", "POST"} {
		path := consentPath
		var b []byte
		if method == "POST" {
			path += "/revoke"
			b = []byte(`{"revision":1}`)
		}
		r, err = p.send(request(ctx, method, path, b))
		if err != nil {
			return err
		}
		if err = p.check("unsigned-"+method, require(r, 401)); err != nil {
			return err
		}
	}
	a, err := p.enroll(ctx)
	if err != nil {
		return fmt.Errorf("synthetic enrollment A: %w", err)
	}
	b, err := p.enroll(ctx)
	if err != nil {
		return fmt.Errorf("synthetic enrollment B: %w", err)
	}
	p.checks = append(p.checks, "two-synthetic-owners-enrolled")
	// If a later check fails after ON, revoke the synthetic decision. A
	// failed recovery is retained in the report, never silently a pass.
	needsOff := false
	defer func() {
		if !needsOff {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, offErr := p.decision(cleanup, a, "/revoke", []byte(`{"revision":2}`), 200, "REVOKED", 2)
		if offErr != nil {
			runErr = errors.Join(runErr, errors.New("synthetic OFF recovery not acknowledged"))
		}
	}()

	v, err := p.read(ctx, a)
	if err != nil {
		return err
	}
	if v["granted"] != false {
		return errors.New("new owner not default OFF")
	}
	p.checks = append(p.checks, "default-off")
	grant := payload(map[string]any{"revision": 1, "terms_version": legal.Version, "terms_digest": legal.Digest})
	needsOff = true
	_, err = p.decision(ctx, a, "/grant", grant, 200, "GRANTED", 1)
	if err = p.check("signed-grant", err); err != nil {
		return err
	}
	v, err = p.read(ctx, b)
	if err != nil {
		return err
	}
	if v["granted"] != false || v["revision"] != nil {
		return errors.New("foreign owner leaked decision")
	}
	p.checks = append(p.checks, "foreign-owner-isolation")
	firstOff := []byte(`{"revision":1}`)
	_, err = p.decision(ctx, b, "/revoke", firstOff, 200, "REVOKED", 1)
	if err = p.check("first-off-without-grant", err); err != nil {
		return err
	}
	v, err = p.decision(ctx, b, "/revoke", firstOff, 200, "REVOKED", 1)
	if err != nil {
		return err
	}
	if v["replayed"] != true || v["terms_version"] != "" {
		return errors.New("first OFF did not replay without reviewed terms")
	}
	p.checks = append(p.checks, "first-off-replay-no-invented-review")
	r, err = p.signed(ctx, a, "POST", consentPath+"/revoke", []byte(`{"revision":2}`), true)
	if err != nil {
		return err
	}
	if err = p.check("forged-signature-refused", require(r, 401)); err != nil {
		return err
	}
	v, err = p.read(ctx, a)
	if err != nil {
		return err
	}
	if v["state"] != "GRANTED" || v["revision"] != float64(1) {
		return errors.New("forged request changed decision")
	}
	p.checks = append(p.checks, "forgery-no-mutation")
	revoke := []byte(`{"revision":2}`)
	_, err = p.decision(ctx, a, "/revoke", revoke, 200, "REVOKED", 2)
	if err = p.check("signed-revoke", err); err != nil {
		return err
	}
	v, err = p.decision(ctx, a, "/revoke", revoke, 200, "REVOKED", 2)
	if err != nil {
		return err
	}
	if v["replayed"] != true {
		return errors.New("OFF replay not idempotent")
	}
	p.checks = append(p.checks, "lost-response-off-replay")
	_, err = p.decision(ctx, a, "/grant", grant, 409, "", 0)
	if err = p.check("stale-on-refused", err); err != nil {
		return err
	}
	_, err = p.decision(ctx, a, "/grant", payload(map[string]any{"revision": 3, "terms_version": legal.Version, "terms_digest": strings.Repeat("0", 64)}), 400, "", 0)
	if err = p.check("wrong-terms-refused", err); err != nil {
		return err
	}
	v, err = p.read(ctx, a)
	if err != nil {
		return err
	}
	if v["granted"] != false || v["state"] != "REVOKED" || v["revision"] != float64(2) {
		return errors.New("final OFF not persistent")
	}
	needsOff = false
	p.checks = append(p.checks, "final-off-persistent")
	return nil
}
