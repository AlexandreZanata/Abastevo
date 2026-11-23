package adapters

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func NewAccountBinding(key []byte, environment, packageName string) (func(string) string, error) {
	if len(key) != 32 || (environment != "test" && environment != "production") || packageName == "" {
		return nil, domain.ErrInvalid
	}
	owned := append([]byte{}, key...)
	return func(accountID string) string {
		if !domain.ValidID(accountID) {
			return ""
		}
		mac := hmac.New(sha256.New, owned)
		mac.Write([]byte("premium-account-v1\x00" + environment + "\x00" + packageName + "\x00" + accountID))
		return hex.EncodeToString(mac.Sum(nil))
	}, nil
}

// NewAppAccountToken derives the StoreKit appAccountToken for an account: a
// UUID (RFC 9562 version 8) made from the same keyed, one-way binding, so
// Apple never learns the account ID (B-BR-STORE-04).
func NewAppAccountToken(key []byte, environment, bundleID string) (func(string) string, error) {
	if len(key) != 32 || (environment != "test" && environment != "production") || bundleID == "" {
		return nil, domain.ErrInvalid
	}
	owned := append([]byte{}, key...)
	return func(accountID string) string {
		if !domain.ValidID(accountID) {
			return ""
		}
		mac := hmac.New(sha256.New, owned)
		mac.Write([]byte("premium-app-account-token-v1\x00" + environment + "\x00" + bundleID + "\x00" + accountID))
		b := mac.Sum(nil)[:16]
		b[6] = (b[6] & 0x0f) | 0x80
		b[8] = (b[8] & 0x3f) | 0x80
		h := hex.EncodeToString(b)
		return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	}, nil
}
