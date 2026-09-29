package profile

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Frozen profile constants (docs/security/identity-profile.md).
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

// stdVerify follows the shared profile helpers.
func stdVerify(v vector, now time.Time) error {
	return Verify(v.JWKX, v.JWKY, v.Lines, v.Signature, now)
}

// independentVerify rebuilds the base by hand (no shared builder) and
// re-derives the fingerprint inline, so a serialization drift in helpers
// cannot pass both paths.
func independentVerify(v vector, now time.Time) error {
	return verifyWith(v.JWKX, v.JWKY, v.Lines, v.Signature, now, func(lines []string) string {
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
