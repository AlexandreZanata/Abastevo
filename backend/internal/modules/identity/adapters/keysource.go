package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/identity"
)

// DeviceKey is one registered contributor key for out-of-band proof
// verification (P13-T04C). The account binding ceremony resolves the
// proof fingerprint through this source instead of importing identity
// tables elsewhere.
type DeviceKey struct {
	ContributorID string
	Fingerprint   string
	JWKX          string
	JWKY          string
	Revoked       bool
}

// FindDeviceKey resolves a key fingerprint to its owner and JWK
// coordinates. Unknown fingerprints report a lookup error; callers map
// every failure to a uniform proof denial so no oracle distinguishes
// unknown from revoked keys.
func (r *Registrar) FindDeviceKey(ctx context.Context, fingerprint string) (DeviceKey, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return DeviceKey{}, fmt.Errorf("keysource: empty fingerprint")
	}
	row, err := identity.New(r.pool).FindKeyByFingerprint(ctx, fingerprint)
	if err != nil {
		return DeviceKey{}, err
	}
	var jwk struct {
		Crv string `json:"crv"`
		Kty string `json:"kty"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	if err := json.Unmarshal([]byte(row.PublicJwk), &jwk); err != nil {
		return DeviceKey{}, fmt.Errorf("keysource: malformed stored JWK")
	}
	if jwk.Crv != "P-256" || jwk.Kty != "EC" || jwk.X == "" || jwk.Y == "" {
		return DeviceKey{}, fmt.Errorf("keysource: unsupported stored key")
	}
	return DeviceKey{
		ContributorID: uuidString(row.ContributorID),
		Fingerprint:   row.Fingerprint,
		JWKX:          jwk.X,
		JWKY:          jwk.Y,
		Revoked:       row.RevokedAt.Valid,
	}, nil
}
