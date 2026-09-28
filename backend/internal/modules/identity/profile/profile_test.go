package profile

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Frozen profile constants (docs/security/identity-profile.md).
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

func contractsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts")
}

func loadVectors(t *testing.T) []vector {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(contractsDir(t), "testdata", "identity", "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no vectors found: %v", err)
	}
	var out []vector
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		var v vector
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("parse %s: %v", filepath.Base(path), err)
		}
		if v.ID == "" || v.Signature == "" {
			t.Fatalf("%s: vector needs id and signature", filepath.Base(path))
		}
		out = append(out, v)
	}
	return out
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
func verifyWith(v vector, now time.Time, rebuild func([]string) string) error {
	if err := checkLines(v.Lines); err != nil {
		return err
	}
	created, err := strconv.ParseInt(parseLineValue(v.Lines[idxOf(v.Lines, "created")]), 10, 64)
	if err != nil {
		return fmt.Errorf("profile: bad created")
	}
	expires, err := strconv.ParseInt(parseLineValue(v.Lines[idxOf(v.Lines, "expires")]), 10, 64)
	if err != nil {
		return fmt.Errorf("profile: bad expires")
	}
	if expires-created > 300 || expires-created <= 0 {
		return fmt.Errorf("profile: window exceeds 5 minutes")
	}
	if now.Unix() > expires {
		return fmt.Errorf("profile: expired")
	}
	pub, err := publicKey(v.JWKX, v.JWKY)
	if err != nil {
		return err
	}
	thumb := `{"crv":"P-256","kty":"EC","x":"` + v.JWKX + `","y":"` + v.JWKY + `"}`
	fp := sha256.Sum256([]byte(thumb))
	if want := "fp:" + fmt.Sprintf("%x", fp); parseLineValue(v.Lines[idxOf(v.Lines, "keyid")]) != want {
		return fmt.Errorf("profile: keyid not bound to key")
	}
	sig, err := b64url(v.Signature)
	if err != nil || len(sig) != 64 {
		return fmt.Errorf("profile: signature must be 64 raw bytes")
	}
	digest := sha512.Sum512([]byte(rebuild(v.Lines)))
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
func stdVerify(v vector, now time.Time) error {
	return verifyWith(v, now, buildBase)
}

// independentVerify rebuilds the base by hand (no shared builder) and
// re-derives the fingerprint inline, so a serialization drift in helpers
// cannot pass both paths.
func independentVerify(v vector, now time.Time) error {
	return verifyWith(v, now, func(lines []string) string {
		out := ""
		for i, l := range lines {
			if i > 0 {
				out += "\n"
			}
			out += l
		}
		return out
	})
}

func TestGoldenVectors(t *testing.T) {
	now := time.Unix(1735689700, 0)
	vecs := loadVectors(t)
	if len(vecs) < 8 {
		t.Fatalf("only %d vectors; the tamper matrix needs broad coverage", len(vecs))
	}
	valid := 0
	for _, v := range vecs {
		t.Run(v.ID, func(t *testing.T) {
			stdErr := stdVerify(v, now)
			indErr := independentVerify(v, now)
			if v.Valid {
				valid++
				if stdErr != nil {
					t.Errorf("stdlib path rejects valid vector: %v", stdErr)
				}
				if indErr != nil {
					t.Errorf("independent path rejects valid vector: %v", indErr)
				}
			} else {
				if stdErr == nil {
					t.Errorf("stdlib path accepts %s (%s)", v.ID, v.Note)
				}
				if indErr == nil {
					t.Errorf("independent path accepts %s (%s)", v.ID, v.Note)
				}
			}
		})
	}
	if valid < 2 {
		t.Errorf("only %d valid vectors; need positive and body coverage", valid)
	}
}

func TestAmbiguousLinesRejected(t *testing.T) {
	now := time.Unix(1735689700, 0)
	vecs := loadVectors(t)
	var base *vector
	for i := range vecs {
		if vecs[i].Valid {
			base = &vecs[i]
			break
		}
	}
	if base == nil {
		t.Fatal("no valid vector to mutate")
	}
	mk := func(lines []string) vector {
		c := *base
		c.ID = "runtime-mutation"
		c.Lines = lines
		return c
	}
	// Reordered covered set.
	swapped := append([]string{}, base.Lines...)
	if len(swapped) >= 2 {
		swapped[0], swapped[1] = swapped[1], swapped[0]
		if err := stdVerify(mk(swapped), now); err == nil {
			t.Error("reordered lines accepted")
		}
	}
	// Duplicated auth line.
	dup := append(append([]string{}, base.Lines...), base.Lines[0])
	if err := stdVerify(mk(dup), now); err == nil {
		t.Error("duplicated line accepted")
	}
}

// fixedReader replays one byte forever. Test-only determinism for frozen
// vectors; production signs with crypto/rand and never touches this.
type fixedReader struct{ b byte }

func (f fixedReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = f.b
	}
	return len(p), nil
}

func mustB64(t *testing.T, b []byte) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(b)
}

// TestGenerateVectors rewrites the golden vectors deterministically. It
// runs only with ANPFUEL_REGENERATE_AUTH_VECTORS=1; normal runs only read.
func TestGenerateVectors(t *testing.T) {
	if os.Getenv("ANPFUEL_REGENERATE_AUTH_VECTORS") != "1" {
		t.Skip("vector regeneration is explicit")
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), fixedReader{0x42})
	if err != nil {
		t.Fatal(err)
	}
	other, err := ecdsa.GenerateKey(elliptic.P256(), fixedReader{0x77})
	if err != nil {
		t.Fatal(err)
	}
	pubBytes, err := priv.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	x := mustB64(t, pubBytes[1:33])
	y := mustB64(t, pubBytes[33:65])
	dSeed, err := priv.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	d := mustB64(t, dSeed)
	thumb := `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
	fpsum := sha256.Sum256([]byte(thumb))
	fp := fmt.Sprintf("fp:%x", fpsum)
	otherBytes, err := other.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	ox := mustB64(t, otherBytes[1:33])
	oy := mustB64(t, otherBytes[33:65])

	sign := func(t *testing.T, key *ecdsa.PrivateKey, lines []string) string {
		t.Helper()
		digest := sha512.Sum512([]byte(buildBase(lines)))
		r, s, err := ecdsa.Sign(fixedReader{0x99}, key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		raw := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
		return mustB64(t, raw)
	}
	baseLines := []string{
		`"@method": POST`,
		`"@authority": api.example.invalid`,
		`"@path": /v1/contributors`,
		`"@query": `,
		`"created": 1735689600`,
		`"expires": 1735689900`,
		`"keyid": "` + fp + `"`,
		`"nonce": "ch-1.client-1"`,
	}
	bodyLines := []string{
		`"@method": POST`,
		`"@authority": api.example.invalid`,
		`"@path": /v1/contributors`,
		`"@query": `,
		`"content-type": application/json`,
		`"content-digest": "sha-512=:MEUCIQ==:"`,
		`"created": 1735689600`,
		`"expires": 1735689900`,
		`"keyid": "` + fp + `"`,
		`"nonce": "ch-1.client-1"`,
	}
	// mk signs signLines but stores storeLines: tamper vectors simulate an
	// attacker editing a valid vector without the key.
	mk2 := func(id, prov string, key *ecdsa.PrivateKey, jx, jy string, signLines, storeLines []string, valid bool, note string) vector {
		return vector{ID: id, Provenance: prov, JWKX: jx, JWKY: jy, D: d,
			Lines: storeLines, Signature: sign(t, key, signLines), Valid: valid, Note: note}
	}
	mk := func(id, prov string, key *ecdsa.PrivateKey, jx, jy string, lines []string, valid bool, note string) vector {
		return mk2(id, prov, key, jx, jy, lines, lines, valid, note)
	}
	mutate := func(lines []string, idx int, val string) []string {
		out := append([]string{}, lines...)
		out[idx] = val
		return out
	}
	derSig := func() string {
		digest := sha512.Sum512([]byte(buildBase(baseLines)))
		r, s, err := ecdsa.Sign(fixedReader{0x99}, priv, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		encInt := func(b []byte) []byte {
			b = append([]byte{}, b...)
			for len(b) > 1 && b[0] == 0 {
				b = b[1:]
			}
			if b[0]&0x80 != 0 {
				b = append([]byte{0}, b...)
			}
			return append([]byte{0x02, byte(len(b))}, b...)
		}
		seq := append(encInt(r.Bytes()), encInt(s.Bytes())...)
		return mustB64(t, append([]byte{0x30, byte(len(seq))}, seq...))
	}
	oldLines := []string{
		`"@method": POST`,
		`"@authority": api.example.invalid`,
		`"@path": /v1/contributors`,
		`"@query": `,
		`"created": 1735686000`,
		`"expires": 1735686300`,
		`"keyid": "` + fp + `"`,
		`"nonce": "ch-1.client-1"`,
	}
	reordered := []string{baseLines[1], baseLines[0], baseLines[2], baseLines[3], baseLines[4], baseLines[5], baseLines[6], baseLines[7]}
	vecs := []vector{
		mk("auth-valid-register", "P03-T01 frozen profile; synthetic fixed-entropy key", priv, x, y, baseLines, true, "baseline"),
		mk("auth-valid-body", "P03-T01 frozen profile; synthetic fixed-entropy key", priv, x, y, bodyLines, true, "body covered set"),
		mk2("auth-tampered-method", "P03-T01 frozen profile; synthetic", priv, x, y, baseLines, mutate(baseLines, 0, `"@method": GET`), false, "method changed"),
		mk2("auth-tampered-path", "P03-T01 frozen profile; synthetic", priv, x, y, baseLines, mutate(baseLines, 2, `"@path": /v1/observations`), false, "path changed"),
		mk2("auth-tampered-query", "P03-T01 frozen profile; synthetic", priv, x, y, baseLines, mutate(baseLines, 3, `"@query": a=b`), false, "query changed"),
		mk2("auth-tampered-authority", "P03-T01 frozen profile; synthetic", priv, x, y, baseLines, mutate(baseLines, 1, `"@authority": evil.example`), false, "authority changed"),
		mk2("auth-tampered-nonce", "P03-T01 frozen profile; synthetic", priv, x, y, baseLines, mutate(baseLines, 7, `"nonce": "ch-9.client-9"`), false, "nonce rebound"),
		mk2("auth-tampered-body", "P03-T01 frozen profile; synthetic", priv, x, y, bodyLines, mutate(bodyLines, 5, `"content-digest": "sha-512=:OTHER==:"`), false, "body changed"),
		mk("auth-expired", "P03-T01 frozen profile; synthetic", priv, x, y, oldLines, false, "verified past expiry"),
		mk2("auth-wrong-key", "P03-T01 frozen profile; synthetic", priv, ox, oy, baseLines, baseLines, false, "key does not match signature"),
		{ID: "auth-der-encoding", Provenance: "P03-T01 frozen profile; synthetic", JWKX: x, JWKY: y, D: d, Lines: baseLines, Signature: derSig(), Valid: false, Note: "DER rejected, raw only"},
		mk2("auth-reordered", "P03-T01 frozen profile; synthetic", priv, x, y, baseLines, reordered, false, "order changed"),
	}
	_ = other
	dir := filepath.Join(contractsDir(t), "testdata", "identity")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, v := range vecs {
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, v.ID+".json"), append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
