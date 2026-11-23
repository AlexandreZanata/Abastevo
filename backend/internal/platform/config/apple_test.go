package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestAppleNativeAudienceDefaultsOff(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppleSignIn.NativeLoginConfigured() || cfg.AppleSignIn.NativeAudience != "" {
		t.Fatalf("native sign-in configured by default: %+v", cfg.AppleSignIn)
	}
	cfg, err = load(envFunc(map[string]string{keyAppleNativeAudience: "   "}))
	if err != nil || cfg.AppleSignIn.NativeLoginConfigured() {
		t.Fatalf("blank audience must stay off: %+v %v", cfg.AppleSignIn, err)
	}
}

func TestAppleNativeAudienceAcceptsBundleID(t *testing.T) {
	cfg, err := load(envFunc(map[string]string{keyAppleNativeAudience: " com.anpfuel.app "}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppleSignIn.NativeAudience != "com.anpfuel.app" || !cfg.AppleSignIn.NativeLoginConfigured() {
		t.Fatalf("audience = %q", cfg.AppleSignIn.NativeAudience)
	}
	if !strings.Contains(fmt.Sprintf("%v", cfg.LogValue()), "apple_native_login_configured=true") {
		t.Fatal("log value must report the native sign-in switch")
	}
}

func TestAppleNativeAudienceRefusesMalformedOrLinkAudience(t *testing.T) {
	for _, bad := range []string{
		"anpfuel-backend",
		"com.anpfuel app",
		".com.anpfuel.app",
		"-com.anpfuel.app",
		"com/anpfuel/app",
		"https://appleid.apple.com",
		strings.Repeat("a", 256),
	} {
		_, err := load(envFunc(map[string]string{keyAppleNativeAudience: bad}))
		if err == nil {
			t.Fatalf("audience %q accepted", bad)
		}
		if !strings.Contains(err.Error(), keyAppleNativeAudience) || strings.Contains(err.Error(), bad) {
			t.Fatalf("error must name the variable, never echo the value: %v", err)
		}
	}
}

func appleRevocationEnv(overrides map[string]string) map[string]string {
	env := map[string]string{
		keyAppleNativeAudience: "com.anpfuel.app",
		keyAppleTeamID:         "TEAM123456",
		keyAppleKeyID:          "KEY1234567",
		keyApplePrivateKeyFile: "/run/secrets/apple/AuthKey.p8",
		keyAppleGrantKeyFile:   "/run/secrets/apple/grant.key",
	}
	for k, v := range overrides {
		env[k] = v
	}
	return env
}

func TestAppleRevocationDefaultsOffAndLoadsTogether(t *testing.T) {
	cfg, err := load(envFunc(map[string]string{keyAppleNativeAudience: "com.anpfuel.app"}))
	if err != nil || cfg.AppleSignIn.RevocationConfigured() {
		t.Fatalf("revocation must default off: %+v %v", cfg.AppleSignIn, err)
	}
	cfg, err = load(envFunc(appleRevocationEnv(nil)))
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.AppleSignIn
	if !a.RevocationConfigured() || a.TeamID != "TEAM123456" || a.KeyID != "KEY1234567" ||
		a.PrivateKeyFile != "/run/secrets/apple/AuthKey.p8" || a.GrantKeyFile != "/run/secrets/apple/grant.key" {
		t.Fatalf("revocation config = %+v", a)
	}
	rendered := fmt.Sprintf("%v", cfg.LogValue())
	if !strings.Contains(rendered, "apple_revocation_configured=true") || strings.Contains(rendered, "/run/secrets") || strings.Contains(rendered, "TEAM123456") {
		t.Fatalf("log value must report only the switch: %s", rendered)
	}
}

func TestAppleRevocationRefusesPartialOrMalformed(t *testing.T) {
	cases := map[string]map[string]string{
		"missing team":           {keyAppleTeamID: ""},
		"missing key id":         {keyAppleKeyID: ""},
		"missing private key":    {keyApplePrivateKeyFile: ""},
		"missing grant key":      {keyAppleGrantKeyFile: ""},
		"no native audience":     {keyAppleNativeAudience: ""},
		"short team":             {keyAppleTeamID: "TEAM12345"},
		"lowercase key id":       {keyAppleKeyID: "key1234567"},
		"relative private key":   {keyApplePrivateKeyFile: "AuthKey.p8"},
		"relative grant key":     {keyAppleGrantKeyFile: "secrets/grant.key"},
		"same file for both":     {keyAppleGrantKeyFile: "/run/secrets/apple/AuthKey.p8"},
		"team with punctuation":  {keyAppleTeamID: "TEAM-23456"},
		"key id with whitespace": {keyAppleKeyID: "KEY 234567"},
	}
	for name, override := range cases {
		_, err := load(envFunc(appleRevocationEnv(override)))
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		if strings.Contains(err.Error(), "/run/secrets") || strings.Contains(err.Error(), "TEAM123456") {
			t.Fatalf("%s: error echoed a value: %v", name, err)
		}
	}
}
