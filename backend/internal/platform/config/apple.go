package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Native Sign in with Apple (P13-T05). The native audience is the client
// identifier Apple writes into the identity token's aud claim for the
// iOS app (its bundle ID). Unset keeps POST /v1/accounts/providers/login
// answering 503 account.provider-unavailable: native sign-in fails
// closed until a deployment names its own client.
//
// Grant revocation (App Store guideline 5.1.1(v)) needs the team ID, the
// Sign in with Apple key ID, the downloaded .p8 private key file and a
// separate 32-byte file that seals stored refresh tokens. The four are
// set together or not at all, and require the native audience, which is
// also the client_id of the token and revoke calls. Without them sign-in
// still works and deletion has nothing to revoke. Key material is read
// from files at process start; only paths live in the environment.
const (
	keyAppleNativeAudience = "ANPFUEL_APPLE_NATIVE_AUDIENCE"
	keyAppleTeamID         = "ANPFUEL_APPLE_TEAM_ID"
	keyAppleKeyID          = "ANPFUEL_APPLE_KEY_ID"
	keyApplePrivateKeyFile = "ANPFUEL_APPLE_PRIVATE_KEY_FILE"
	keyAppleGrantKeyFile   = "ANPFUEL_APPLE_GRANT_KEY_FILE"
)

// linkAudience is the frozen server-side audience of provider-link
// tokens. A native audience equal to it would let one token class
// stand in for the other, so configuration refuses it.
const linkAudience = "anpfuel-backend"

// AppleSignInConfig carries the native Sign in with Apple switches. IDs
// are public identifiers; the key files hold the secrets and are never
// read or logged here.
type AppleSignInConfig struct {
	// NativeAudience is the expected identity-token audience (bundle
	// ID); empty disables native sign-in.
	NativeAudience string
	// TeamID and KeyID are Apple's 10-character identifiers.
	TeamID string
	KeyID  string
	// PrivateKeyFile is the absolute path of the .p8 key; GrantKeyFile
	// the absolute path of the 32-byte grant sealing key.
	PrivateKeyFile string
	GrantKeyFile   string
}

// NativeLoginConfigured reports whether native sign-in may run.
func (c AppleSignInConfig) NativeLoginConfigured() bool {
	return c.NativeAudience != ""
}

// RevocationConfigured reports whether grants are captured and revoked.
func (c AppleSignInConfig) RevocationConfigured() bool {
	return c.NativeAudience != "" && c.TeamID != "" && c.KeyID != "" && c.PrivateKeyFile != "" && c.GrantKeyFile != ""
}

func loadApple(getenv func(string) (string, bool)) (AppleSignInConfig, error) {
	var cfg AppleSignInConfig
	raw, _ := getenv(keyAppleNativeAudience)
	audience := strings.TrimSpace(raw)
	if audience != "" {
		if !validAppleClientID(audience) || audience == linkAudience {
			return AppleSignInConfig{}, fmt.Errorf("%s must be the app bundle ID (letters, digits, '.' and '-') and differ from the link audience", keyAppleNativeAudience)
		}
		cfg.NativeAudience = audience
	}
	values := map[string]string{}
	set := 0
	for _, key := range []string{keyAppleTeamID, keyAppleKeyID, keyApplePrivateKeyFile, keyAppleGrantKeyFile} {
		raw, _ := getenv(key)
		values[key] = strings.TrimSpace(raw)
		if values[key] != "" {
			set++
		}
	}
	if set == 0 {
		return cfg, nil
	}
	if set != 4 {
		return AppleSignInConfig{}, fmt.Errorf("%s, %s, %s and %s must be set together", keyAppleTeamID, keyAppleKeyID, keyApplePrivateKeyFile, keyAppleGrantKeyFile)
	}
	if cfg.NativeAudience == "" {
		return AppleSignInConfig{}, fmt.Errorf("%s is required for Apple grant revocation", keyAppleNativeAudience)
	}
	for _, key := range []string{keyAppleTeamID, keyAppleKeyID} {
		if !validAppleIdentifier(values[key]) {
			return AppleSignInConfig{}, fmt.Errorf("%s must be a 10-character Apple identifier", key)
		}
	}
	for _, key := range []string{keyApplePrivateKeyFile, keyAppleGrantKeyFile} {
		if !filepath.IsAbs(values[key]) {
			return AppleSignInConfig{}, fmt.Errorf("%s must be an absolute path", key)
		}
	}
	if filepath.Clean(values[keyApplePrivateKeyFile]) == filepath.Clean(values[keyAppleGrantKeyFile]) {
		return AppleSignInConfig{}, fmt.Errorf("%s and %s must be different files", keyApplePrivateKeyFile, keyAppleGrantKeyFile)
	}
	cfg.TeamID = values[keyAppleTeamID]
	cfg.KeyID = values[keyAppleKeyID]
	cfg.PrivateKeyFile = values[keyApplePrivateKeyFile]
	cfg.GrantKeyFile = values[keyAppleGrantKeyFile]
	return cfg, nil
}

// validAppleClientID accepts Apple App ID / Services ID shapes:
// 1..255 ASCII letters, digits, '.' and '-', starting with a letter or
// digit.
func validAppleClientID(id string) bool {
	if len(id) == 0 || len(id) > 255 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case (c == '.' || c == '-') && i > 0:
		default:
			return false
		}
	}
	return true
}

// validAppleIdentifier accepts Apple's 10-character team and key IDs:
// uppercase letters and digits.
func validAppleIdentifier(id string) bool {
	if len(id) != 10 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
