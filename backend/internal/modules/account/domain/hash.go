package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"math/big"
)

// aliasAlphabet drops ambiguous glyphs (0/o, 1/i/l) so aliases read aloud.
const aliasAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// SHA256Hasher implements CodeHasher with salted SHA-256 and constant-time
// comparison. Codes are short numeric strings; the salt carries the entropy
// that makes the stored verifier useless without the code itself.
type SHA256Hasher struct{}

// NewSalt returns 16 crypto/rand bytes hex-encoded for storage.
func (SHA256Hasher) NewSalt() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Hash derives the stored verifier for salt+code.
func (SHA256Hasher) Hash(salt, code string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + code))
	return hex.EncodeToString(sum[:])
}

// Equal compares in constant time over the decoded hash bytes.
func (SHA256Hasher) Equal(storedHash, salt, code string) bool {
	want, err1 := hex.DecodeString(storedHash)
	got, err2 := hex.DecodeString(SHA256Hasher{}.Hash(salt, code))
	if err1 != nil || err2 != nil || len(want) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare(want, got) == 1
}

// EqualHash compares stored hex verifiers without byte-prefix timing
// oracles. Both stores share it so memory and SQL paths leak identically
// nothing.
func EqualHash(a, b string) bool {
	ra, err1 := hex.DecodeString(a)
	rb, err2 := hex.DecodeString(b)
	if err1 != nil || err2 != nil || len(ra) != len(rb) {
		return false
	}
	return subtle.ConstantTimeCompare(ra, rb) == 1
}

// GenerateCode mints a uniform zero-padded CodeDigits number from crypto/rand.
func GenerateCode() (string, error) {
	top := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, top)
	if err != nil {
		return "", err
	}
	s := n.String()
	for len(s) < CodeDigits {
		s = "0" + s
	}
	return s, nil
}

// GenerateToken mints 32 crypto/rand bytes hex-encoded for session tokens.
func GenerateToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// GenerateAlias mints an opaque public alias like "k7q2-m9zx-42ab".
func GenerateAlias() (string, error) {
	var letters [12]byte
	for i := range letters {
		var b [1]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		letters[i] = aliasAlphabet[int(b[0])%len(aliasAlphabet)]
	}
	s := string(letters[:])
	return s[0:4] + "-" + s[4:8] + "-" + s[8:12], nil
}
