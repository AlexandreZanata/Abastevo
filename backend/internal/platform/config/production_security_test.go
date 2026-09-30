package config

import (
	"strings"
	"testing"
)

func TestProductionRequiresStableAuthorityAndCursorKey(t *testing.T) {
	good := map[string]string{keyEnv: "production", keyDatabaseURL: "postgres://app:synthetic@db/app?sslmode=require", keyCanonicalHost: "fuel.example.org", keyCursorSecret: strings.Repeat("a", 32)}
	for _, key := range []string{keyCanonicalHost, keyCursorSecret} {
		t.Run(key, func(t *testing.T) {
			vars := map[string]string{}
			for k, v := range good {
				vars[k] = v
			}
			delete(vars, key)
			if _, err := load(envFunc(vars)); err == nil {
				t.Fatal("production accepted missing security configuration")
			}
		})
	}
	for _, host := range []string{"api.example.invalid", "bad\nhost", "https://fuel.example.org", ".invalid"} {
		vars := map[string]string{}
		for k, v := range good {
			vars[k] = v
		}
		vars[keyCanonicalHost] = host
		if _, err := load(envFunc(vars)); err == nil {
			t.Error("production accepted invalid authority")
		}
	}
	if _, err := load(envFunc(good)); err != nil {
		t.Fatal(err)
	}
}
