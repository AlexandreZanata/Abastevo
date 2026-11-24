package domain

import (
	"crypto/subtle"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrNameInvalid              = errors.New("account: invalid public name")
	ErrEmailInvalid             = errors.New("account: invalid email")
	ErrRecoveryNoAccount        = errors.New("account: no account for verified address")
	ErrReauthenticationRequired = errors.New("account: fresh authentication required")
	ErrEmailLoginRetired        = errors.New("account: email sign-in retired")
)

// Sensitive changes require a session created within this window. Refresh
// rotates credentials without changing the original authentication time.
const FreshLoginSeconds = 600

const (
	RecoveryAuditTTLSeconds = 180 * 24 * 3600
	RecoveryAction          = "recovery"
	RecoveryEmailAdd        = "email_add"
	RecoveryEmailChange     = "email_change"
)

// RecoveryAddress contains only the lookup hash and the owner-visible hint.
type RecoveryAddress struct {
	Hash string
	Hint string
}

func EmailHint(address string) string {
	local, host, ok := strings.Cut(NormalizeAddress(address), "@")
	if !ok || local == "" {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(local)
	return string(first) + "***@" + host
}

// NormalizePublicName canonicalizes spelling and spacing before persistence.
func NormalizePublicName(name string) string {
	return norm.NFC.String(strings.Join(strings.Fields(name), " "))
}

func ValidPublicName(name string) bool {
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	name = NormalizePublicName(name)
	if size := utf8.RuneCountInString(name); size < 2 || size > 40 {
		return false
	}
	if containsReservedWord(name) {
		return false
	}
	letters, digits := 0, 0
	for _, r := range name {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			digits++
		case unicode.IsMark(r), r == ' ', r == '\'', r == '’', r == '-', r == '.':
		default:
			return false
		}
	}
	return letters >= 2 && digits <= 4
}

// reservedNameWords cannot appear as a word anywhere in a public name, so
// "Suporte Abastevo" cannot pose as the service on public author lines.
var reservedNameWords = map[string]bool{
	"abastevo": true, "admin": true, "administrador": true, "moderador": true,
	"moderacao": true, "moderação": true, "suporte": true, "anp": true, "oficial": true,
}

// containsReservedWord splits the normalized name into runs of letters and
// marks; digits and punctuation separate words.
func containsReservedWord(name string) bool {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r)
	})
	for _, word := range words {
		if reservedNameWords[word] {
			return true
		}
	}
	return false
}

func (a Account) PublicName() string {
	if a.DisplayName != "" {
		return a.DisplayName
	}
	return a.Alias
}

// SessionProof captures the verified salted access verifier. Stores recheck
// it under the write lock so a rotated or revoked family cannot mutate
// profile or recovery data while the request is in flight.
type SessionProof struct {
	FamilyID   string
	AccountID  string
	AccessHash string
	Now        int64
}

func (p SessionProof) Verify(f SessionFamily, fresh bool) error {
	if f.ID != p.FamilyID || f.AccountID != p.AccountID {
		return ErrCodeUnknown
	}
	if f.RevokedAt != 0 {
		return ErrSessionRevoked
	}
	if p.Now > f.AccessExpires {
		return ErrSessionExpired
	}
	if subtle.ConstantTimeCompare([]byte(f.AccessHash), []byte(p.AccessHash)) != 1 {
		return ErrCodeUnknown
	}
	if fresh && (f.IssuedAt > p.Now || p.Now-f.IssuedAt > FreshLoginSeconds) {
		return ErrReauthenticationRequired
	}
	return nil
}

// AccountProfile contains the private profile fields shown to its owner.
type AccountProfile struct {
	Account           Account
	RecoveryEmailHint string
	HasAccountKey     bool
	Providers         []string
}
