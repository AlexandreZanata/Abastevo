package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountSMTPConfigRequiresEveryValueAndReadableSecret(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "smtp.secret")
	const secret = "synthetic-smtp-secret"
	if err := os.WriteFile(secretFile, []byte(secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"ANPFUEL_MAIL_MODE": "smtp", "ANPFUEL_SMTP_HOST": "smtp.example.invalid", "ANPFUEL_SMTP_PORT": "587",
		"ANPFUEL_SMTP_TLS": "starttls", "ANPFUEL_SMTP_USERNAME": "test-user@example.invalid",
		"ANPFUEL_SMTP_PASSWORD_FILE": secretFile, "ANPFUEL_SMTP_FROM": "alerts@example.invalid",
		"ANPFUEL_MAIL_MAX_CODES_PER_HOUR": "2000",
	}
	cfg, err := load(envFunc(values))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MailMode != "smtp" || cfg.SMTP.Port != 587 || cfg.SMTP.Password != secret || cfg.MailMaxCodesPerHour != 2000 {
		t.Fatal("configured SMTP values did not load")
	}
	var logged bytes.Buffer
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("loaded", "config", cfg)
	for _, private := range []string{secret, values["ANPFUEL_SMTP_FROM"], values["ANPFUEL_SMTP_USERNAME"], secretFile} {
		if strings.Contains(logged.String(), private) {
			t.Error("config log leaked SMTP data")
		}
	}
	for _, key := range []string{"ANPFUEL_SMTP_HOST", "ANPFUEL_SMTP_PORT", "ANPFUEL_SMTP_TLS", "ANPFUEL_SMTP_USERNAME", "ANPFUEL_SMTP_PASSWORD_FILE", "ANPFUEL_SMTP_FROM"} {
		old := values[key]
		delete(values, key)
		_, err := load(envFunc(values))
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("missing %s must refuse startup: %v", key, err)
		}
		values[key] = old
	}
	values["ANPFUEL_SMTP_PASSWORD_FILE"] += ".missing"
	if _, err := load(envFunc(values)); err == nil || strings.Contains(err.Error(), secretFile) {
		t.Errorf("unreadable file error must redact path: %v", err)
	}
}

func TestMailAggregateLimitBounds(t *testing.T) {
	for _, raw := range []string{"", "20", "2000", "19", "2001", "garbage"} {
		cfg, err := load(envFunc(map[string]string{"ANPFUEL_MAIL_MAX_CODES_PER_HOUR": raw}))
		switch raw {
		case "":
			if err != nil || cfg.MailMaxCodesPerHour != 200 {
				t.Fatalf("default = %d, err = %v", cfg.MailMaxCodesPerHour, err)
			}
		case "20", "2000":
			if err != nil {
				t.Fatalf("bound %s refused: %v", raw, err)
			}
		default:
			if err == nil {
				t.Fatalf("invalid limit %q accepted", raw)
			}
		}
	}
}

func TestGoogleNativeConfigSeparatesAudienceAuthorizedPartiesAndLegacyLink(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil || cfg.GoogleSignIn.NativeAudience != "" || len(cfg.GoogleSignIn.AuthorizedParties) != 0 {
		t.Fatal("Google native sign-in must default off")
	}
	values := map[string]string{
		"ANPFUEL_GOOGLE_NATIVE_AUDIENCE":    "test-web.apps.example.invalid",
		"ANPFUEL_GOOGLE_AUTHORIZED_PARTIES": "test-android.apps.example.invalid, test-upload.apps.example.invalid",
	}
	cfg, err = load(envFunc(values))
	if err != nil || cfg.GoogleSignIn.NativeAudience != values["ANPFUEL_GOOGLE_NATIVE_AUDIENCE"] ||
		!cfg.GoogleSignIn.AuthorizedParties["test-android.apps.example.invalid"] || !cfg.GoogleSignIn.AuthorizedParties["test-upload.apps.example.invalid"] {
		t.Fatalf("Google audience or authorized parties did not load: %v", err)
	}
	values["ANPFUEL_GOOGLE_NATIVE_AUDIENCE"] = "anpfuel-backend"
	if _, err := load(envFunc(values)); err == nil {
		t.Error("native audience cannot alias the legacy link audience")
	}
}
