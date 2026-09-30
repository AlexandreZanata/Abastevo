package profile

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProofTimeWindowRejectsFutureAndExpiredBoundary(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	x := base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32)))
	y := base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))
	fp, err := Thumbprint(x, y)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	for _, tc := range []struct {
		name             string
		created, expires int64
		denied           bool
	}{{"valid", now.Unix() - 1, now.Unix() + 60, false}, {"future", now.Unix() + 3600, now.Unix() + 3660, true}, {"expiry-equality", now.Unix() - 60, now.Unix(), true}, {"overflow", -9223372036854775808, 9223372036854775807, true}} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{`"@method": POST`, `"@authority": test.invalid`, `"@path": /v1/contributors`, `"@query": `, fmt.Sprintf(`"created": %d`, tc.created), fmt.Sprintf(`"expires": %d`, tc.expires), `"keyid": "` + fp + `"`, `"nonce": "challenge.nonce"`}
			sum := sha512.Sum512([]byte(strings.Join(lines, "\n")))
			r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
			if err != nil {
				t.Fatal(err)
			}
			sig := base64.RawURLEncoding.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
			err = Verify(x, y, lines, sig, now)
			if (err != nil) != tc.denied {
				t.Fatalf("denied=%v, err=%v", tc.denied, err)
			}
		})
	}
}
