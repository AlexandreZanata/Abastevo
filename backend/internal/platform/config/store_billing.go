package config

import (
	"encoding/base64"
	"log/slog"
	"os"
	"sort"
	"strings"
)

// Store billing (B-BR-STORE-01..10): Google Play and App Store in-app
// subscriptions. Each store channel stays off until its credentials and
// catalog are complete; a partial configuration records issues and keeps
// the channel off instead of failing startup. Secrets are read from mounted
// files or base64 values and are never logged.
const (
	keyStoreEnvironment       = "ANPFUEL_STORE_ENVIRONMENT"
	keyStoreTokenKey          = "ANPFUEL_STORE_TOKEN_KEY"
	keyStoreBindingKey        = "ANPFUEL_STORE_BINDING_KEY"
	keyPlayPackageName        = "ANPFUEL_PLAY_PACKAGE_NAME"
	keyPlayServiceAccountFile = "ANPFUEL_PLAY_SERVICE_ACCOUNT_FILE"
	keyPlayProducts           = "ANPFUEL_PLAY_PRODUCTS"
	keyPlayPushSubscription   = "ANPFUEL_PLAY_PUSH_SUBSCRIPTION"
	keyPlayPushAudience       = "ANPFUEL_PLAY_PUSH_AUDIENCE"
	keyPlayPushServiceAccount = "ANPFUEL_PLAY_PUSH_SERVICE_ACCOUNT"
	keyAppStoreBundleID       = "ANPFUEL_APPSTORE_BUNDLE_ID"
	keyAppStoreIssuerID       = "ANPFUEL_APPSTORE_ISSUER_ID"
	keyAppStoreKeyID          = "ANPFUEL_APPSTORE_KEY_ID"
	keyAppStorePrivateKeyFile = "ANPFUEL_APPSTORE_PRIVATE_KEY_FILE"
	keyAppStoreProducts       = "ANPFUEL_APPSTORE_PRODUCTS"
	keyAppStoreSandbox        = "ANPFUEL_APPSTORE_SANDBOX_FALLBACK"
	defaultStoreAppID         = "com.anpfuel.app"
)

// readSecretFile is replaced in tests.
var readSecretFile = os.ReadFile

type PlayStoreConfig struct {
	PackageName        string
	ServiceAccountJSON []byte
	// Products maps a subscription product ID to its sellable base plans.
	Products           map[string][]string
	PushSubscription   string
	PushAudience       string
	PushServiceAccount string
	Configured         bool
	// PushConfigured reports a complete RTDN push identity.
	PushConfigured bool
}

type AppStoreConfig struct {
	BundleID        string
	IssuerID        string
	KeyID           string
	PrivateKeyPEM   []byte
	Products        []string
	SandboxFallback bool
	Configured      bool
}

type StoreBillingConfig struct {
	// Environment is "test" or "production"; empty keeps both stores off.
	Environment string
	TokenKey    []byte
	BindingKey  []byte
	Play        PlayStoreConfig
	AppStore    AppStoreConfig
	Issues      []string
}

func loadStoreBilling(getenv func(string) (string, bool)) StoreBillingConfig {
	cfg := StoreBillingConfig{
		Play:     PlayStoreConfig{PackageName: defaultStoreAppID},
		AppStore: AppStoreConfig{BundleID: defaultStoreAppID},
	}
	text := func(key string) string {
		raw, _ := getenv(key)
		return strings.TrimSpace(raw)
	}
	issue := func(message string) { cfg.Issues = append(cfg.Issues, message) }
	if v := text(keyPlayPackageName); v != "" {
		cfg.Play.PackageName = v
	}
	if v := text(keyAppStoreBundleID); v != "" {
		cfg.AppStore.BundleID = v
	}
	playRequested := text(keyPlayServiceAccountFile) != "" || text(keyPlayProducts) != ""
	appleRequested := text(keyAppStoreIssuerID) != "" || text(keyAppStoreKeyID) != "" || text(keyAppStorePrivateKeyFile) != "" || text(keyAppStoreProducts) != ""
	if !playRequested && !appleRequested {
		return cfg
	}
	common := true
	switch env := text(keyStoreEnvironment); env {
	case "test", "production":
		cfg.Environment = env
	default:
		issue(keyStoreEnvironment + " must be test or production")
		common = false
	}
	cfg.TokenKey = storeKey(text(keyStoreTokenKey))
	if cfg.TokenKey == nil {
		issue(keyStoreTokenKey + " must be 32 bytes in base64")
		common = false
	}
	cfg.BindingKey = storeKey(text(keyStoreBindingKey))
	if cfg.BindingKey == nil {
		issue(keyStoreBindingKey + " must be 32 bytes in base64")
		common = false
	}

	if playRequested {
		ok := common
		if !validStoreID(cfg.Play.PackageName) {
			issue(keyPlayPackageName + " is malformed")
			ok = false
		}
		cfg.Play.Products = parsePlayProducts(text(keyPlayProducts))
		if len(cfg.Play.Products) == 0 {
			issue(keyPlayProducts + " must list product:basePlan[,basePlan] entries separated by ;")
			ok = false
		}
		if raw, err := readSecretFile(text(keyPlayServiceAccountFile)); err != nil || len(raw) == 0 || len(raw) > 16<<10 {
			issue(keyPlayServiceAccountFile + " must name a readable service account JSON key")
			ok = false
		} else {
			cfg.Play.ServiceAccountJSON = raw
		}
		cfg.Play.Configured = ok
		cfg.Play.PushSubscription = text(keyPlayPushSubscription)
		cfg.Play.PushAudience = text(keyPlayPushAudience)
		cfg.Play.PushServiceAccount = text(keyPlayPushServiceAccount)
		push := cfg.Play.PushSubscription != "" || cfg.Play.PushAudience != "" || cfg.Play.PushServiceAccount != ""
		if push && (!strings.HasPrefix(cfg.Play.PushSubscription, "projects/") || cfg.Play.PushAudience == "" || !strings.Contains(cfg.Play.PushServiceAccount, "@")) {
			issue(keyPlayPushSubscription + ", " + keyPlayPushAudience + " and " + keyPlayPushServiceAccount + " must be set together")
		} else {
			cfg.Play.PushConfigured = push && ok
		}
	}

	if appleRequested {
		ok := common
		if !validStoreID(cfg.AppStore.BundleID) {
			issue(keyAppStoreBundleID + " is malformed")
			ok = false
		}
		cfg.AppStore.IssuerID = text(keyAppStoreIssuerID)
		cfg.AppStore.KeyID = text(keyAppStoreKeyID)
		if !validPaymentText(cfg.AppStore.IssuerID, 64) || !validPaymentText(cfg.AppStore.KeyID, 32) {
			issue(keyAppStoreIssuerID + " and " + keyAppStoreKeyID + " are required")
			ok = false
		}
		for _, product := range strings.Split(text(keyAppStoreProducts), ",") {
			if product = strings.TrimSpace(product); product != "" && validStoreID(product) {
				cfg.AppStore.Products = append(cfg.AppStore.Products, product)
			}
		}
		sort.Strings(cfg.AppStore.Products)
		if len(cfg.AppStore.Products) == 0 {
			issue(keyAppStoreProducts + " must list product IDs separated by commas")
			ok = false
		}
		if raw, err := readSecretFile(text(keyAppStorePrivateKeyFile)); err != nil || len(raw) == 0 || len(raw) > 4<<10 {
			issue(keyAppStorePrivateKeyFile + " must name a readable .p8 key")
			ok = false
		} else {
			cfg.AppStore.PrivateKeyPEM = raw
		}
		cfg.AppStore.SandboxFallback = paymentBool(getenv, keyAppStoreSandbox, false, &cfg.Issues)
		cfg.AppStore.Configured = ok
	}
	return cfg
}

func storeKey(raw string) []byte {
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil
	}
	return key
}

func validStoreID(value string) bool {
	if value == "" || len(value) > 150 {
		return false
	}
	for _, c := range value {
		if !(c == '.' || c == '_' || c == '-' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')) {
			return false
		}
	}
	return true
}

// parsePlayProducts reads "product:plan1,plan2;product2:plan".
func parsePlayProducts(raw string) map[string][]string {
	products := map[string][]string{}
	for _, entry := range strings.Split(raw, ";") {
		product, plans, found := strings.Cut(strings.TrimSpace(entry), ":")
		if !found || !validStoreID(strings.TrimSpace(product)) {
			continue
		}
		var bases []string
		for _, plan := range strings.Split(plans, ",") {
			if plan = strings.TrimSpace(plan); validStoreID(plan) {
				bases = append(bases, plan)
			}
		}
		if len(bases) > 0 {
			products[strings.TrimSpace(product)] = bases
		}
	}
	return products
}

func (s StoreBillingConfig) logValue() slog.Value {
	return slog.GroupValue(
		slog.String("environment", s.Environment),
		slog.Bool("play_configured", s.Play.Configured),
		slog.Bool("play_push_configured", s.Play.PushConfigured),
		slog.Int("play_products", len(s.Play.Products)),
		slog.Bool("app_store_configured", s.AppStore.Configured),
		slog.Int("app_store_products", len(s.AppStore.Products)),
		slog.Bool("app_store_sandbox_fallback", s.AppStore.SandboxFallback),
		slog.Int("issues", len(s.Issues)),
	)
}
