package profile

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

var coveredOrder = []string{
	"@method", "@authority", "@path", "@query",
	"content-type", "content-digest",
	"created", "expires", "keyid", "nonce",
}

type vector struct {
	ID         string   `json:"id"`
	Provenance string   `json:"provenance"`
	JWKX       string   `json:"jwk_x"`
	JWKY       string   `json:"jwk_y"`
	D          string   `json:"private_d"`
	Lines      []string `json:"base_lines"`
	Signature  string   `json:"signature"`
	Valid      bool     `json:"valid"`
	Note       string   `json:"note"`
}

func b64url(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// buildBase reconstructs the canonical base from ordered lines.
func buildBase(lines []string) string {
	return strings.Join(lines, "\n")
}

// publicKey parses the JWK coordinates onto P-256, refusing off-curve and
// degenerate points.
func publicKey(xb, yb string) (*ecdsa.PublicKey, error) {
	x, err := b64url(xb)
	if err != nil {
		return nil, err
	}
	y, err := b64url(yb)
	if err != nil {
		return nil, err
	}
	if len(x) != 32 || len(y) != 32 {
		return nil, fmt.Errorf("profile: JWK coordinates must be 32 bytes")
	}
	bigX, bigY := new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)
	if bigX.Sign() == 0 && bigY.Sign() == 0 {
		return nil, fmt.Errorf("profile: degenerate key")
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append([]byte{0x04}, append(x, y...)...))
	if err != nil {
		return nil, fmt.Errorf("profile: key off curve")
	}
	return pub, nil
}

// checkLines enforces the exact covered set and order: no missing, extra,
// reordered or duplicated lines.
func checkLines(lines []string) error {
	wantBody := false
	for _, l := range lines {
		if strings.HasPrefix(l, `"content-type":`) || strings.HasPrefix(l, `"content-digest":`) {
			wantBody = true
		}
	}
	var want []string
	for _, name := range coveredOrder {
		if (name == "content-type" || name == "content-digest") && !wantBody {
			continue
		}
		want = append(want, `"`+name+`":`)
	}
	if len(lines) != len(want) {
		return fmt.Errorf("profile: %d lines, want %d", len(lines), len(want))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, want[i]) {
			return fmt.Errorf("profile: line %d not %s", i, want[i])
		}
	}
	return nil
}

func parseLineValue(line string) string {
	_, v, _ := strings.Cut(line, ": ")
	return strings.Trim(v, `"`)
}

// verifyWith checks time window, key binding, base shape and signature.
func verifyWith(jwkX, jwkY string, lines []string, signature string, now time.Time, rebuild func([]string) string) error {
	if err := checkLines(lines); err != nil {
		return err
	}
	created, err := strconv.ParseInt(parseLineValue(lines[idxOf(lines, "created")]), 10, 64)
	if err != nil {
		return fmt.Errorf("profile: bad created")
	}
	expires, err := strconv.ParseInt(parseLineValue(lines[idxOf(lines, "expires")]), 10, 64)
	if err != nil {
		return fmt.Errorf("profile: bad expires")
	}
	if expires-created > 300 || expires-created <= 0 {
		return fmt.Errorf("profile: window exceeds 5 minutes")
	}
	if now.Unix() > expires {
		return fmt.Errorf("profile: expired")
	}
	pub, err := publicKey(jwkX, jwkY)
	if err != nil {
		return err
	}
	thumb := `{"crv":"P-256","kty":"EC","x":"` + jwkX + `","y":"` + jwkY + `"}`
	fp := sha256.Sum256([]byte(thumb))
	if want := "fp:" + fmt.Sprintf("%x", fp); parseLineValue(lines[idxOf(lines, "keyid")]) != want {
		return fmt.Errorf("profile: keyid not bound to key")
	}
	sig, err := b64url(signature)
	if err != nil || len(sig) != 64 {
		return fmt.Errorf("profile: signature must be 64 raw bytes")
	}
	digest := sha512.Sum512([]byte(rebuild(lines)))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return fmt.Errorf("profile: bad signature")
	}
	return nil
}

func idxOf(lines []string, name string) int {
	for i, l := range lines {
		if strings.HasPrefix(l, `"`+name+`":`) {
			return i
		}
	}
	return -1
}

// stdVerify follows the shared profile helpers.

// Thumbprint returns the "fp:<hex>" key identifier for JWK coordinates,
// validating the point. Adapters use it to bind challenges and keys to the
// exact verified key, never to a body-supplied fingerprint.
func Thumbprint(jwkX, jwkY string) (string, error) {
	if _, err := publicKey(jwkX, jwkY); err != nil {
		return "", err
	}
	thumb := `{"crv":"P-256","kty":"EC","x":"` + jwkX + `","y":"` + jwkY + `"}`
	fp := sha256.Sum256([]byte(thumb))
	return "fp:" + fmt.Sprintf("%x", fp), nil
}

// Verify checks a proof against the frozen profile using the shared helpers.
func Verify(jwkX, jwkY string, lines []string, signature string, now time.Time) error {
	return verifyWith(jwkX, jwkY, lines, signature, now, buildBase)
}
