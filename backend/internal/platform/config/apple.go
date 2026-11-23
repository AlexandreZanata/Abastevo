package config

import (
	"fmt"
	"strings"
)

// Native Sign in with Apple (P13-T05). The native audience is the client
// identifier Apple writes into the identity token's aud claim for the
// iOS app (its bundle ID). Unset keeps POST /v1/accounts/providers/login
// answering 503 account.provider-unavailable: native sign-in fails
// closed until a deployment names its own client.
const keyAppleNativeAudience = "ANPFUEL_APPLE_NATIVE_AUDIENCE"

// linkAudience is the frozen server-side audience of provider-link
// tokens. A native audience equal to it would let one token class
// stand in for the other, so configuration refuses it.
const linkAudience = "anpfuel-backend"

// AppleSignInConfig carries the native Sign in with Apple switches. The
// values are public identifiers, never secrets.
type AppleSignInConfig struct {
	// NativeAudience is the expected identity-token audience (bundle
	// ID); empty disables native sign-in.
	NativeAudience string
}

// NativeLoginConfigured reports whether native sign-in may run.
func (c AppleSignInConfig) NativeLoginConfigured() bool {
	return c.NativeAudience != ""
}

func loadApple(getenv func(string) (string, bool)) (AppleSignInConfig, error) {
	var cfg AppleSignInConfig
	raw, _ := getenv(keyAppleNativeAudience)
	audience := strings.TrimSpace(raw)
	if audience == "" {
		return cfg, nil
	}
	if !validAppleClientID(audience) || audience == linkAudience {
		return AppleSignInConfig{}, fmt.Errorf("%s must be the app bundle ID (letters, digits, '.' and '-') and differ from the link audience", keyAppleNativeAudience)
	}
	cfg.NativeAudience = audience
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
