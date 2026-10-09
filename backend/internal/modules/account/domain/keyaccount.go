package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strings"
)

// Anonymous key-account credential (no email, no provider, no device key).
// The username is display + login handle; the account key is the ONLY
// secret. The key itself never touches storage, logs or errors: the
// salted verifier (Hash) authenticates, the unsalted Lookup finds the
// candidate row without an oracle (unknown and wrong keys share
// ErrKeyInvalid).
type KeyCredential struct {
	AccountID    string
	UsernameHash string
	KeyLookup    string
	KeySalt      string
	KeyHash      string
	CreatedAt    int64
}

// keyAlphabet reuses the unambiguous alias glyphs (no 0/o, 1/i/l) so keys
// survive manual transcription; 31 glyphs over KeyLength positions carry
// ~158 bits of entropy.
const keyAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// KeyLength is the canonical account-key length.
const KeyLength = 32

// NormalizeUsername trims and lowercases for hashing and lookup. Lookup
// keys are always hashes of this form, never raw usernames.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// ValidUsername reports whether username is a creatable handle: 3-20
// lowercase alphanumerics starting with a letter.
func ValidUsername(username string) bool {
	name := NormalizeUsername(username)
	if len(name) < 3 || len(name) > 20 {
		return false
	}
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// UsernameHash derives the PII-minimized lookup key for a username.
func UsernameHash(username string) string {
	sum := sha256.Sum256([]byte(NormalizeUsername(username)))
	return hex.EncodeToString(sum[:])
}

// GenerateAccountKey mints one uniform KeyLength-glyph key from
// crypto/rand. Uniqueness across accounts comes from 158 bits of entropy
// plus the unique key_lookup constraint with retry on collision.
func GenerateAccountKey() (string, error) {
	top := big.NewInt(int64(len(keyAlphabet)))
	var sb strings.Builder
	sb.Grow(KeyLength)
	for i := 0; i < KeyLength; i++ {
		n, err := rand.Int(rand.Reader, top)
		if err != nil {
			return "", err
		}
		sb.WriteByte(keyAlphabet[n.Int64()])
	}
	return sb.String(), nil
}

// CanonicalKey strips transcription separators (spaces, dashes) and
// lowercases. Anything outside the alphabet or length fails closed with
// ErrKeyInvalid, identical to an unknown key.
func CanonicalKey(raw string) (string, error) {
	var sb strings.Builder
	sb.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r == ' ' || r == '-' || r == '_':
			continue
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r - 'A' + 'a')
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r)
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		default:
			return "", ErrKeyInvalid
		}
	}
	key := sb.String()
	if len(key) != KeyLength {
		return "", ErrKeyInvalid
	}
	for i := 0; i < len(key); i++ {
		if !strings.ContainsRune(keyAlphabet, rune(key[i])) {
			return "", ErrKeyInvalid
		}
	}
	return key, nil
}

// KeyLookup derives the unsalted index for a canonical key. Lookup finds
// the candidate row; the salted Hash still authenticates.
func KeyLookup(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}
