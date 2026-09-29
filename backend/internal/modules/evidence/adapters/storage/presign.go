package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// Bounds for short direct-upload authorizations. The URL TTL caps at 15
// min: least-privilege short credentials, far below the 24 h session
// reservation deadline. Zero TTL selects DefaultURLTTL.
const (
	DefaultURLTTL = 5 * time.Minute
	MinURLTTL     = time.Minute
	MaxURLTTL     = 15 * time.Minute
	// QuarantinePrefix scopes every client-writable key. Final sanitized
	// objects live under a different prefix the worker writes with server
	// credentials only — never through a presigned URL from this package.
	QuarantinePrefix = "q/"
	maxKeyBytes      = 1024
	defaultRegion    = "auto"
)

var (
	ErrBadEndpoint    = errors.New("storage: endpoint must be http(s) with a host")
	ErrBadBucket      = errors.New("storage: bucket must be 3..63 lowercase alphanumerics, dots or dashes")
	ErrBadKey         = errors.New("storage: key must live under the quarantine namespace without traversal")
	ErrBadContentType = errors.New("storage: only the session media type is presignable")
	ErrBadSize        = errors.New("storage: bound outside 1..session maximum")
	ErrBadTTL         = errors.New("storage: TTL outside 1..15 minutes")
	ErrBadCredentials = errors.New("storage: access key and secret are required")
)

// Credentials carries the server-side storage identity. There is
// deliberately no String or LogValue method: secrets never format into
// logs, errors or URLs.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

// PresignInput describes exactly one intended upload. Every field is
// validated before any URL exists; the minted URL binds method, key,
// content type and expiry together under the signature.
type PresignInput struct {
	Endpoint    string
	Bucket      string
	Key         string
	ContentType string
	MaxBytes    int64
	TTL         time.Duration
	Region      string
	Now         time.Time
}

// PresignedPUT is the short authorization handed to the client: the URL
// plus the headers the signature covers. Content-Length is intentionally
// absent: presigned transport cannot bound the body (proven by test), so
// the declared MaxBytes travels in session metadata and the worker
// verifies actual length afterward (P05-T03).
type PresignedPUT struct {
	URL             string
	RequiredHeaders map[string]string
	ExpiresAt       time.Time
	MaxBytes        int64
}

// PresignPUT mints one SigV4 query-authenticated PUT URL with payload
// UNSIGNED-PAYLOAD (R2/S3 compatible). The bucket stays private: no ACL
// or grant parameter is ever emitted, and keys outside the quarantine
// namespace are refused before signing.
func PresignPUT(in PresignInput, cred Credentials) (PresignedPUT, error) {
	if strings.TrimSpace(cred.AccessKeyID) == "" || cred.SecretAccessKey == "" {
		return PresignedPUT{}, ErrBadCredentials
	}
	endpoint, err := parseEndpoint(in.Endpoint)
	if err != nil {
		return PresignedPUT{}, err
	}
	if !validBucket(in.Bucket) {
		return PresignedPUT{}, ErrBadBucket
	}
	if !validQuarantineKey(in.Key) {
		return PresignedPUT{}, ErrBadKey
	}
	if in.ContentType != domain.AllowedMIME {
		return PresignedPUT{}, ErrBadContentType
	}
	if in.MaxBytes < 1 || in.MaxBytes > domain.MaxUploadBytes {
		return PresignedPUT{}, ErrBadSize
	}
	ttl := in.TTL
	if ttl == 0 {
		ttl = DefaultURLTTL
	}
	if ttl < MinURLTTL || ttl > MaxURLTTL {
		return PresignedPUT{}, ErrBadTTL
	}
	region := in.Region
	if strings.TrimSpace(region) == "" {
		region = defaultRegion
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	credentialScope := dateStamp + "/" + region + "/s3/aws4_request"
	signedHeaders := "content-type;host"

	canonicalURI := "/" + in.Bucket + "/" + encodeKey(in.Key)
	query := map[string]string{
		"X-Amz-Algorithm":     "AWS4-HMAC-SHA256",
		"X-Amz-Credential":    cred.AccessKeyID + "/" + credentialScope,
		"X-Amz-Date":          amzDate,
		"X-Amz-Expires":       fmt.Sprintf("%d", int64(ttl/time.Second)),
		"X-Amz-SignedHeaders": signedHeaders,
	}
	canonicalQuery := canonicalQueryString(query)
	canonicalHeaders := "content-type:" + strings.TrimSpace(in.ContentType) + "\n" + "host:" + endpoint.Host + "\n"
	payloadHash := sha256.Sum256([]byte("PUT\n" + canonicalURI + "\n" + canonicalQuery + "\n" + canonicalHeaders + "\n" + signedHeaders + "\nUNSIGNED-PAYLOAD"))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" + hex.EncodeToString(payloadHash[:])
	signature := hex.EncodeToString(sign(deriveSigningKey(cred.SecretAccessKey, dateStamp, region), stringToSign))

	rawURL := strings.TrimSuffix(endpoint.String(), "/") + canonicalURI + "?" + canonicalQuery + "&X-Amz-Signature=" + signature
	return PresignedPUT{
		URL:             rawURL,
		RequiredHeaders: map[string]string{"Content-Type": in.ContentType},
		ExpiresAt:       now.Add(ttl),
		MaxBytes:        in.MaxBytes,
	}, nil
}

// parseEndpoint accepts https anywhere and http for loopback only, so a
// local S3 emulator works in development while staging and production
// stay TLS-only. Credentials in the endpoint URL are refused.
func parseEndpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return nil, ErrBadEndpoint
	}
	switch u.Scheme {
	case "https":
	case "http":
		host := u.Hostname()
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return nil, ErrBadEndpoint
		}
	default:
		return nil, ErrBadEndpoint
	}
	return u, nil
}

func validBucket(b string) bool {
	if len(b) < 3 || len(b) > 63 {
		return false
	}
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' {
			continue
		}
		return false
	}
	if b[0] == '.' || b[0] == '-' || b[len(b)-1] == '.' || b[len(b)-1] == '-' {
		return false
	}
	return true
}

func validQuarantineKey(k string) bool {
	if k == "" || len(k) > maxKeyBytes {
		return false
	}
	if !strings.HasPrefix(k, QuarantinePrefix) || strings.HasPrefix(k, "/") {
		return false
	}
	if strings.Contains(k, "..") || strings.Contains(k, "\\") {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x20 || k[i] == 0x7f {
			return false
		}
	}
	return true
}

// encodeKey escapes one object key per RFC 3986, preserving the slash
// separators between segments.
func encodeKey(k string) string {
	segs := strings.Split(k, "/")
	for i, s := range segs {
		segs[i] = encodeComponent(s)
	}
	return strings.Join(segs, "/")
}

func encodeComponent(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	return e
}

func canonicalQueryString(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(encodeComponent(k))
		b.WriteByte('=')
		b.WriteString(encodeComponent(params[k]))
	}
	return b.String()
}

func hmacSHA256(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

func deriveSigningKey(secret, date, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}

func sign(key []byte, stringToSign string) []byte {
	return hmacSHA256(key, stringToSign)
}
