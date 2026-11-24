package config

import (
	"fmt"
	"strings"
)

const (
	keyGoogleNativeAudience    = "ANPFUEL_GOOGLE_NATIVE_AUDIENCE"
	keyGoogleAuthorizedParties = "ANPFUEL_GOOGLE_AUTHORIZED_PARTIES"
)

// GoogleSignInConfig separates the native Web client audience from the
// Android client IDs allowed to present its tokens.
type GoogleSignInConfig struct {
	NativeAudience    string
	AuthorizedParties map[string]bool
}

func loadGoogle(getenv func(string) (string, bool)) (GoogleSignInConfig, error) {
	var cfg GoogleSignInConfig
	rawAudience, _ := getenv(keyGoogleNativeAudience)
	audience := strings.TrimSpace(rawAudience)
	if audience != "" && (audience == linkAudience || !validGoogleClientID(audience)) {
		return cfg, fmt.Errorf("%s must be a Google client ID distinct from the link audience", keyGoogleNativeAudience)
	}
	cfg.NativeAudience = audience
	rawParties, _ := getenv(keyGoogleAuthorizedParties)
	if strings.TrimSpace(rawParties) != "" {
		cfg.AuthorizedParties = make(map[string]bool)
		for _, raw := range strings.Split(rawParties, ",") {
			party := strings.TrimSpace(raw)
			if !validGoogleClientID(party) {
				return GoogleSignInConfig{}, fmt.Errorf("%s must be comma-separated Google client IDs", keyGoogleAuthorizedParties)
			}
			cfg.AuthorizedParties[party] = true
		}
	}
	return cfg, nil
}

func validGoogleClientID(id string) bool {
	if len(id) == 0 || len(id) > 255 {
		return false
	}
	return !strings.ContainsFunc(id, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
	})
}
