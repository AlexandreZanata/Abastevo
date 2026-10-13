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
