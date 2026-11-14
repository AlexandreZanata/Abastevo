package adapters

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
)

// InvitationSecretFactory derives a distinct verifier key; raw credentials are
// generated per request and never passed to persistence. Key rotation requires
// reissue. The returned function has no reversible token cache.
func InvitationSecretFactory(serverSecret []byte) (func(kernel.OrgScope, string) (app.InvitationSecret, error), error) {
	if len(serverSecret) < 32 {
		return nil, app.ErrRegisterUnavailable
	}
	derive := hmac.New(sha256.New, serverSecret)
	_, _ = derive.Write([]byte("abastevo.enterprise.invitation.key.v1"))
	key := derive.Sum(nil)
	return func(scope kernel.OrgScope, id string) (app.InvitationSecret, error) {
		parsed, e := kernel.ParseOrgID(id)
		if scope.Validate() != nil || e != nil || parsed.String() != id {
			return app.InvitationSecret{}, app.ErrRegisterUnavailable
		}
		raw := make([]byte, 32)
		if _, e := rand.Read(raw); e != nil {
			return app.InvitationSecret{}, app.ErrRegisterUnavailable
		}
		secret := base64.RawURLEncoding.EncodeToString(raw)
		mac := hmac.New(sha256.New, key)
		_, _ = fmt.Fprintf(mac, "invitation.verifier.v1:%s:%s:%s", scope.OrgID().String(), id, secret)
		return app.InvitationSecret{Code: "AIN1." + id + "." + secret, Verifier: hex.EncodeToString(mac.Sum(nil))}, nil
	}, nil
}
