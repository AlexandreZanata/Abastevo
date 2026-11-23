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
