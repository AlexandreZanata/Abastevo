package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

// rfc3986 encodes one query component per the SigV4 rules: QueryEscape
// with space as %20 (never +) and no reliance on package helpers, so this
// verifier stays an independent reimplementation of the spec.
func rfc3986(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	return e
}

// canonicalQuerySpec rebuilds the canonical query string straight from the
// received URL, excluding the signature itself.
func canonicalQuerySpec(u *url.URL) string {
	vals := u.Query()
	vals.Del("X-Amz-Signature")
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(rfc3986(k))
		b.WriteByte('=')
		b.WriteString(rfc3986(vals.Get(k)))
	}
	return b.String()
}

// specKey derives the signing key with raw HMAC primitives, mirroring the
// AWS documentation rather than any package helper.
func specKey(secret, date, region string, toSign string) string {
	h := func(key []byte, s string) []byte {
		m := hmac.New(sha256.New, key)
		m.Write([]byte(s))
		return m.Sum(nil)
	}
	kDate := h([]byte("AWS4"+secret), date)
	kRegion := h(kDate, region)
	kService := h(kRegion, "s3")
	kSigning := h(kService, "aws4_request")
	m := hmac.New(sha256.New, kSigning)
	m.Write([]byte(toSign))
	return hex.EncodeToString(m.Sum(nil))
}

// fakeS3 verifies SigV4 query auth like a private S3-compatible store:
// exact-key binding, signed-header enforcement, expiry against an injected
// clock and no public grants. Bodies are accepted at any length on
// purpose: the suite asserts presigning does NOT enforce size.
type fakeS3 struct {
	t          *testing.T
	secret     string
	access     string
	bucket     string
	key        string
	now        time.Time
	lastLength int64
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.EscapedPath() != "/"+f.bucket+"/"+f.key {
		http.Error(w, "key", http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	for _, k := range []string{"X-Amz-Algorithm", "X-Amz-Credential", "X-Amz-Date", "X-Amz-Expires", "X-Amz-SignedHeaders", "X-Amz-Signature"} {
		if q.Get(k) == "" {
			http.Error(w, "missing "+k, http.StatusForbidden)
			return
		}
	}
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		http.Error(w, "algorithm", http.StatusForbidden)
		return
	}
	cred, err := url.QueryUnescape(q.Get("X-Amz-Credential"))
	if err != nil {
		http.Error(w, "credential", http.StatusForbidden)
		return
	}
	parts := strings.Split(cred, "/")
	if len(parts) != 5 || parts[0] != f.access || parts[3] != "s3" || parts[4] != "aws4_request" {
		http.Error(w, "credential scope", http.StatusForbidden)
		return
	}
	amzTime, err := time.Parse("20060102T150405Z", q.Get("X-Amz-Date"))
	if err != nil || parts[1] != q.Get("X-Amz-Date")[:8] {
		http.Error(w, "date", http.StatusForbidden)
		return
	}
	ttl, err := time.ParseDuration(q.Get("X-Amz-Expires") + "s")
	if err != nil {
		http.Error(w, "expires", http.StatusForbidden)
		return
	}
	if !f.now.Before(amzTime.Add(ttl)) {
		http.Error(w, "expired", http.StatusForbidden)
		return
	}
	signed := strings.Split(q.Get("X-Amz-SignedHeaders"), ";")
	var canonicalHeaders strings.Builder
	for _, h := range signed {
		switch h {
		case "host":
			fmt.Fprintf(&canonicalHeaders, "host:%s\n", r.Host)
		case "content-type":
			fmt.Fprintf(&canonicalHeaders, "content-type:%s\n", strings.TrimSpace(r.Header.Get("Content-Type")))
		default:
			http.Error(w, "signed header", http.StatusForbidden)
			return
		}
	}
	canonical := "PUT\n" + r.URL.EscapedPath() + "\n" + canonicalQuerySpec(r.URL) + "\n" +
		canonicalHeaders.String() + "\n" + strings.Join(signed, ";") + "\nUNSIGNED-PAYLOAD"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + q.Get("X-Amz-Date") + "\n" + parts[1] + "/" + parts[2] + "/s3/aws4_request\n" + hex.EncodeToString(sum[:])
	want := specKey(f.secret, parts[1], parts[2], toSign)
	if subtle.ConstantTimeCompare([]byte(want), []byte(strings.ToLower(q.Get("X-Amz-Signature")))) != 1 {
		http.Error(w, "signature", http.StatusForbidden)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, "body", http.StatusBadRequest)
		return
	}
	f.lastLength = int64(len(raw))
	digest := sha256.Sum256(raw)
	w.Header().Set("ETag", `"`+hex.EncodeToString(digest[:])+`"`)
	w.WriteHeader(http.StatusOK)
}

func testClock() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }

func testInput(endpoint string) PresignInput {
	return PresignInput{
		Endpoint: endpoint, Bucket: "anpfuel-quarantine",
		Key:         "q/e0000000000000000000000000000001",
		ContentType: "image/jpeg", MaxBytes: 3 << 20,
		TTL: 5 * time.Minute, Region: "auto", Now: testClock(),
	}
}

func testCred() Credentials {
	return Credentials{AccessKeyID: "test-access", SecretAccessKey: "test-secret-32-bytes-long-value!"}
}

func put(t *testing.T, rawURL, contentType string, body []byte) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, rawURL, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestPresignedPUTRoundTrip(t *testing.T) {
	fake := &fakeS3{t: t, secret: testCred().SecretAccessKey, access: testCred().AccessKeyID, bucket: "anpfuel-quarantine", key: "q/e0000000000000000000000000000001", now: testClock()}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	defer srv.Close()
	got, err := PresignPUT(testInput(srv.URL), testCred())
	if err != nil {
		t.Fatalf("presign = %v", err)
	}
	if !got.ExpiresAt.Equal(testClock().Add(5 * time.Minute)) {
		t.Errorf("expires = %v", got.ExpiresAt)
	}
	if got.RequiredHeaders["Content-Type"] != "image/jpeg" || got.MaxBytes != 3<<20 {
		t.Errorf("presigned = %+v", got)
	}
	body := make([]byte, 512<<10)
	if code := put(t, got.URL, "image/jpeg", body); code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200", code)
	}
	if fake.lastLength != int64(len(body)) {
		t.Errorf("stored length = %d", fake.lastLength)
	}
}

func TestPresignRejectsBadInput(t *testing.T) {
	base := testInput("https://xxx.r2.cloudflarestorage.com")
	cases := map[string]func(*PresignInput){
		"endpoint-scheme": func(p *PresignInput) { p.Endpoint = "ftp://x" },
		"endpoint-host":   func(p *PresignInput) { p.Endpoint = "https://" },
		"bucket-case":     func(p *PresignInput) { p.Bucket = "UPPER" },
		"bucket-short":    func(p *PresignInput) { p.Bucket = "ab" },
		"key-absolute":    func(p *PresignInput) { p.Key = "/q/x" },
		"key-traverse":    func(p *PresignInput) { p.Key = "q/../x" },
		"key-namespace":   func(p *PresignInput) { p.Key = "final/x" },
		"content-type":    func(p *PresignInput) { p.ContentType = "image/png" },
		"size-zero":       func(p *PresignInput) { p.MaxBytes = 0 },
		"size-over":       func(p *PresignInput) { p.MaxBytes = (3 << 20) + 1 },
		"ttl-short":       func(p *PresignInput) { p.TTL = time.Second },
		"ttl-long":        func(p *PresignInput) { p.TTL = time.Hour },
	}
	for name, mutate := range cases {
		in := base
		mutate(&in)
		if _, err := PresignPUT(in, testCred()); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := PresignPUT(base, Credentials{}); err == nil {
		t.Error("empty credentials accepted")
	}
	// Zero TTL selects the documented 5-minute default.
	in := base
	in.TTL = 0
	got, err := PresignPUT(in, testCred())
	if err != nil {
		t.Fatalf("default TTL: %v", err)
	}
	if !got.ExpiresAt.Equal(testClock().Add(5 * time.Minute)) {
		t.Errorf("default expiry = %v", got.ExpiresAt)
	}
}

func TestExpiredURLRefused(t *testing.T) {
	fake := &fakeS3{t: t, secret: testCred().SecretAccessKey, access: testCred().AccessKeyID, bucket: "anpfuel-quarantine", key: "q/e0000000000000000000000000000001", now: testClock()}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	defer srv.Close()
	got, err := PresignPUT(testInput(srv.URL), testCred())
	if err != nil {
		t.Fatal(err)
	}
	fake.now = testClock().Add(5*time.Minute + time.Second)
	if code := put(t, got.URL, "image/jpeg", []byte("x")); code != http.StatusForbidden {
		t.Errorf("expired PUT = %d, want 403", code)
	}
}

func TestWrongKeyAndTamperingRefused(t *testing.T) {
	fake := &fakeS3{t: t, secret: testCred().SecretAccessKey, access: testCred().AccessKeyID, bucket: "anpfuel-quarantine", key: "q/e0000000000000000000000000000001", now: testClock()}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	defer srv.Close()
	got, err := PresignPUT(testInput(srv.URL), testCred())
	if err != nil {
		t.Fatal(err)
	}
	// Another owner's object key is not covered by this signature.
	other := strings.Replace(got.URL, "e0000000000000000000000000000001", "f0000000000000000000000000000002", 1)
	if code := put(t, other, "image/jpeg", []byte("x")); code != http.StatusForbidden {
		t.Errorf("foreign key PUT = %d, want 403", code)
	}
	// The signed content type is enforced exactly.
	if code := put(t, got.URL, "image/png", []byte("x")); code != http.StatusForbidden {
		t.Errorf("tampered content-type PUT = %d, want 403", code)
	}
	// A flipped signature bit fails verification.
	broken := got.URL[:len(got.URL)-1] + "0"
	if code := put(t, broken, "image/jpeg", []byte("x")); code != http.StatusForbidden {
		t.Errorf("tampered signature PUT = %d, want 403", code)
	}
}

func TestPresignDoesNotEnforceSize(t *testing.T) {
	// Documents the task risk explicitly: the URL cannot bound the body.
	// The worker (P05-T03) verifies actual length afterward and deletes
	// oversize objects; the session MaxBytes travels in metadata only.
	fake := &fakeS3{t: t, secret: testCred().SecretAccessKey, access: testCred().AccessKeyID, bucket: "anpfuel-quarantine", key: "q/e0000000000000000000000000000001", now: testClock()}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	defer srv.Close()
	got, err := PresignPUT(testInput(srv.URL), testCred())
	if err != nil {
		t.Fatal(err)
	}
	huge := make([]byte, 4<<20)
	if code := put(t, got.URL, "image/jpeg", huge); code != http.StatusOK {
		t.Fatalf("oversize PUT = %d, want 200 (unenforced by design)", code)
	}
	if fake.lastLength != int64(len(huge)) {
		t.Errorf("stored length = %d", fake.lastLength)
	}
}

func TestPresignDeterministicAndPrivate(t *testing.T) {
	a, err := PresignPUT(testInput("https://xxx.r2.cloudflarestorage.com"), testCred())
	if err != nil {
		t.Fatal(err)
	}
	b, err := PresignPUT(testInput("https://xxx.r2.cloudflarestorage.com"), testCred())
	if err != nil {
		t.Fatal(err)
	}
	if a.URL != b.URL {
		t.Error("same inputs minted different URLs")
	}
	// The bucket stays private: no ACL grants anywhere in the grant.
	low := strings.ToLower(a.URL)
	for _, token := range []string{"acl", "grant", "public", "policy"} {
		if strings.Contains(low, token) {
			t.Errorf("authorization leaks %q", token)
		}
	}
	for _, h := range []string{"X-Amz-Acl", "X-Amz-Grant-Full-Control", "X-Amz-Grant-Read"} {
		if _, ok := a.RequiredHeaders[h]; ok {
			t.Errorf("required header grants access: %s", h)
		}
	}
}
