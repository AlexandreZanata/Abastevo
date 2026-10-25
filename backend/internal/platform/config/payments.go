package config

import (
	"encoding/hex"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Payment configuration is deliberately separate from business enablement:
// missing or mixed provider credentials disable only the unsafe sale channels.
// New sales default OFF; maintenance defaults ON so accepted access, webhook
// intake, cancellation and reconciliation are not erased by a sales switch.
// No value in this file is a real credential or causes provider I/O.
const (
	keyPaymentsCatalogVersion = "ANPFUEL_PAYMENTS_CATALOG_VERSION"
	keyPaymentsReturnOrigin   = "ANPFUEL_PAYMENTS_RETURN_ORIGIN"

	keyStripeMode        = "ANPFUEL_STRIPE_MODE"
	keyStripeSecret      = "ANPFUEL_STRIPE_SECRET_KEY"
	keyStripeWebhook     = "ANPFUEL_STRIPE_WEBHOOK_SECRET"
	keyStripeMerchant    = "ANPFUEL_STRIPE_MERCHANT_ID"
	keyStripePrice       = "ANPFUEL_STRIPE_PRICE_ID"
	keyStripePortal      = "ANPFUEL_STRIPE_PORTAL_CONFIGURATION_ID"
	keyStripeNewSales    = "ANPFUEL_STRIPE_CARD_NEW_SALES_ENABLED"
	keyStripeMaintenance = "ANPFUEL_STRIPE_MAINTENANCE_ENABLED"

	keyAsaasMode                  = "ANPFUEL_ASAAS_MODE"
	keyAsaasAPIKey                = "ANPFUEL_ASAAS_API_KEY"
	keyAsaasWebhook               = "ANPFUEL_ASAAS_WEBHOOK_TOKEN"
	keyAsaasMerchant              = "ANPFUEL_ASAAS_MERCHANT_ID"
	keyAsaasManualNewSales        = "ANPFUEL_ASAAS_PIX_MANUAL_NEW_SALES_ENABLED"
	keyAsaasAutomaticNewSales     = "ANPFUEL_ASAAS_PIX_AUTOMATIC_NEW_SALES_ENABLED"
	keyAsaasMaintenance           = "ANPFUEL_ASAAS_MAINTENANCE_ENABLED"
	defaultPaymentsCatalogVersion = "pay-catalog-v1"
)

const (
	PaymentReasonDisabledByFlag      = "disabled-by-flag"
	PaymentReasonProviderMissing     = "provider-not-configured"
	PaymentReasonMaintenanceDisabled = "maintenance-disabled"
	PaymentReasonModeMismatch        = "mode-mismatch"
	PaymentReasonMissingCredentials  = "missing-credentials"
	PaymentReasonMissingReturnOrigin = "missing-return-origin"
	PaymentReasonMissingCatalog      = "missing-catalog"
	PaymentReasonInvalidConfig       = "invalid-configuration"
)

type StripePaymentConfig struct {
	Mode               string
	ExpectedMode       string
	SecretKey          string
	WebhookSecret      string
	MerchantID         string
	PriceID            string
	PortalConfigID     string
	NewSalesEnabled    bool
	MaintenanceEnabled bool
	Configured         bool
}

type AsaasPaymentConfig struct {
	Mode                        string
	ExpectedMode                string
	APIKey                      string
	WebhookToken                string
	MerchantID                  string
	PixManualNewSalesEnabled    bool
	PixAutomaticNewSalesEnabled bool
	MaintenanceEnabled          bool
	Configured                  bool
}

type PaymentsConfig struct {
	CatalogVersion string
	ReturnOrigin   string
	Stripe         StripePaymentConfig
	Asaas          AsaasPaymentConfig
	StripeIssues   []string
	AsaasIssues    []string
	CommonIssues   []string
}

type PaymentChannelPreflight struct {
	Channel                string   `json:"channel"`
	NewSalesRequested      bool     `json:"new_sales_requested"`
	NewSalesReady          bool     `json:"new_sales_ready"`
	MaintenanceEnabled     bool     `json:"maintenance_enabled"`
	MaintenanceReady       bool     `json:"maintenance_ready"`
	MaintenanceReason      string   `json:"maintenance_reason,omitempty"`
	Mode                   string   `json:"mode,omitempty"`
	MerchantConfigured     bool     `json:"merchant_configured"`
	WebhookConfigured      bool     `json:"webhook_configured"`
	PriceConfigured        bool     `json:"price_configured,omitempty"`
	PortalConfigured       bool     `json:"portal_configured,omitempty"`
	CatalogConfigured      bool     `json:"catalog_configured"`
	ReturnOriginConfigured bool     `json:"return_origin_configured,omitempty"`
	Reason                 string   `json:"reason,omitempty"`
	Issues                 []string `json:"issues,omitempty"`
}

type PaymentsPreflight struct {
	CatalogVersion string                  `json:"catalog_version"`
	Stripe         PaymentChannelPreflight `json:"stripe_card"`
	PixManual      PaymentChannelPreflight `json:"asaas_pix_manual"`
	PixAutomatic   PaymentChannelPreflight `json:"asaas_pix_automatic"`
}

func loadPayments(getenv func(string) (string, bool), env Environment) PaymentsConfig {
	cfg := PaymentsConfig{
		CatalogVersion: defaultPaymentsCatalogVersion,
		Stripe: StripePaymentConfig{
			ExpectedMode:       expectedStripeMode(env),
			MaintenanceEnabled: true,
		},
		Asaas: AsaasPaymentConfig{
			ExpectedMode:       expectedAsaasMode(env),
			MaintenanceEnabled: true,
		},
	}
	if raw, present := getenv(keyPaymentsCatalogVersion); present {
		cfg.CatalogVersion = strings.TrimSpace(raw)
		if !validPaymentText(cfg.CatalogVersion, 128) {
			cfg.CommonIssues = append(cfg.CommonIssues, keyPaymentsCatalogVersion+" must be a non-empty version")
			cfg.CatalogVersion = ""
		}
	}
	rawOrigin, _ := getenv(keyPaymentsReturnOrigin)
	origin, originOK := validPaymentOrigin(rawOrigin, env)
	if originOK {
		cfg.ReturnOrigin = origin
	}

	cfg.Stripe.NewSalesEnabled = paymentBool(getenv, keyStripeNewSales, false, &cfg.StripeIssues)
	cfg.Stripe.MaintenanceEnabled = paymentBool(getenv, keyStripeMaintenance, true, &cfg.StripeIssues)
	cfg.Stripe.Mode, _ = getenv(keyStripeMode)
	cfg.Stripe.Mode = strings.TrimSpace(cfg.Stripe.Mode)
	if raw, present := getenv(keyStripeSecret); present {
		cfg.Stripe.SecretKey = raw
	}
	if raw, present := getenv(keyStripeWebhook); present {
		cfg.Stripe.WebhookSecret = raw
	}
	if raw, present := getenv(keyStripeMerchant); present {
		cfg.Stripe.MerchantID = strings.TrimSpace(raw)
	}
	if raw, present := getenv(keyStripePrice); present {
		cfg.Stripe.PriceID = strings.TrimSpace(raw)
	}
	if raw, present := getenv(keyStripePortal); present {
		cfg.Stripe.PortalConfigID = strings.TrimSpace(raw)
	}
	cfg.Stripe.Configured = cfg.Stripe.Mode != "" || cfg.Stripe.SecretKey != "" ||
		cfg.Stripe.WebhookSecret != "" || cfg.Stripe.MerchantID != "" ||
		cfg.Stripe.PriceID != "" || cfg.Stripe.NewSalesEnabled
	if cfg.Stripe.Configured {
		if !validStripeMode(cfg.Stripe.Mode) {
			cfg.StripeIssues = append(cfg.StripeIssues, keyStripeMode+" must be test or live")
		} else if cfg.Stripe.Mode != cfg.Stripe.ExpectedMode {
			cfg.StripeIssues = append(cfg.StripeIssues, keyStripeMode+" does not match the platform environment")
		}
		if !validStripeSecret(cfg.Stripe.SecretKey, cfg.Stripe.Mode) {
			cfg.StripeIssues = append(cfg.StripeIssues, keyStripeSecret+" is missing or has the wrong test/live prefix")
		}
		if !validStripeWebhook(cfg.Stripe.WebhookSecret) {
			cfg.StripeIssues = append(cfg.StripeIssues, keyStripeWebhook+" is missing or malformed")
		}
		if !strings.HasPrefix(cfg.Stripe.MerchantID, "acct_") || len(cfg.Stripe.MerchantID) <= 5 {
			cfg.StripeIssues = append(cfg.StripeIssues, keyStripeMerchant+" is missing or malformed")
		}
		if !strings.HasPrefix(cfg.Stripe.PriceID, "price_") || len(cfg.Stripe.PriceID) <= 6 {
			cfg.StripeIssues = append(cfg.StripeIssues, keyStripePrice+" is missing or malformed")
		}
		if cfg.Stripe.PortalConfigID != "" && !validStripePortal(cfg.Stripe.PortalConfigID) {
			cfg.StripeIssues = append(cfg.StripeIssues, keyStripePortal+" is malformed")
		}
		if cfg.Stripe.NewSalesEnabled && !originOK {
			cfg.StripeIssues = append(cfg.StripeIssues, keyPaymentsReturnOrigin+" is required for card sales")
		}
	}

	cfg.Asaas.PixManualNewSalesEnabled = paymentBool(getenv, keyAsaasManualNewSales, false, &cfg.AsaasIssues)
	cfg.Asaas.PixAutomaticNewSalesEnabled = paymentBool(getenv, keyAsaasAutomaticNewSales, false, &cfg.AsaasIssues)
	cfg.Asaas.MaintenanceEnabled = paymentBool(getenv, keyAsaasMaintenance, true, &cfg.AsaasIssues)
	cfg.Asaas.Mode, _ = getenv(keyAsaasMode)
	cfg.Asaas.Mode = strings.TrimSpace(cfg.Asaas.Mode)
	if raw, present := getenv(keyAsaasAPIKey); present {
		cfg.Asaas.APIKey = raw
	}
	if raw, present := getenv(keyAsaasWebhook); present {
		cfg.Asaas.WebhookToken = raw
	}
	if raw, present := getenv(keyAsaasMerchant); present {
		cfg.Asaas.MerchantID = strings.TrimSpace(raw)
	}
	cfg.Asaas.Configured = cfg.Asaas.Mode != "" || cfg.Asaas.APIKey != "" ||
		cfg.Asaas.WebhookToken != "" || cfg.Asaas.MerchantID != "" ||
		cfg.Asaas.PixManualNewSalesEnabled || cfg.Asaas.PixAutomaticNewSalesEnabled
	if cfg.Asaas.Configured {
		if !validAsaasMode(cfg.Asaas.Mode) {
			cfg.AsaasIssues = append(cfg.AsaasIssues, keyAsaasMode+" must be test or production")
		} else if cfg.Asaas.Mode != cfg.Asaas.ExpectedMode {
			cfg.AsaasIssues = append(cfg.AsaasIssues, keyAsaasMode+" does not match the platform environment")
		}
		if !validAsaasAPIKey(cfg.Asaas.APIKey) {
			cfg.AsaasIssues = append(cfg.AsaasIssues, keyAsaasAPIKey+" is missing or malformed")
		}
		if !validAsaasWebhook(cfg.Asaas.WebhookToken) {
			cfg.AsaasIssues = append(cfg.AsaasIssues, keyAsaasWebhook+" is missing or malformed")
		}
		if cfg.Asaas.APIKey != "" && cfg.Asaas.WebhookToken == cfg.Asaas.APIKey {
			cfg.AsaasIssues = append(cfg.AsaasIssues, keyAsaasWebhook+" must not reuse the API key")
		}
		if !validPaymentUUID(cfg.Asaas.MerchantID) {
			cfg.AsaasIssues = append(cfg.AsaasIssues, keyAsaasMerchant+" is missing or malformed")
		}
	}
	sort.Strings(cfg.StripeIssues)
	sort.Strings(cfg.AsaasIssues)
	sort.Strings(cfg.CommonIssues)
	return cfg
}

func (p PaymentsConfig) Preflight() PaymentsPreflight {
	catalogOK := p.CatalogVersion != ""
	stripeMaintenance := p.Stripe.MaintenanceEnabled && stripeCredentialsReady(p.Stripe)
	stripeSales := stripeMaintenance && validStripePrice(p.Stripe.PriceID) && p.ReturnOrigin != "" && catalogOK
	asaasMaintenance := p.Asaas.MaintenanceEnabled && asaasCredentialsReady(p.Asaas)
	manualSales := asaasMaintenance && p.Asaas.PixManualNewSalesEnabled && catalogOK
	autoSales := asaasMaintenance && p.Asaas.PixAutomaticNewSalesEnabled && catalogOK

	commonIssues := append([]string(nil), p.CommonIssues...)
	return PaymentsPreflight{
		CatalogVersion: p.CatalogVersion,
		Stripe: PaymentChannelPreflight{
			Channel: "stripe_card", NewSalesRequested: p.Stripe.NewSalesEnabled,
			NewSalesReady:      p.Stripe.NewSalesEnabled && stripeSales,
			MaintenanceEnabled: p.Stripe.MaintenanceEnabled, MaintenanceReady: stripeMaintenance,
			MaintenanceReason: maintenanceReason(p.Stripe.MaintenanceEnabled, p.Stripe.Configured,
				p.Stripe.Mode == p.Stripe.ExpectedMode, stripeCredentialsReady(p.Stripe)),
			Mode: p.Stripe.Mode, MerchantConfigured: strings.HasPrefix(p.Stripe.MerchantID, "acct_"),
			WebhookConfigured:      validStripeWebhook(p.Stripe.WebhookSecret),
			PriceConfigured:        validStripePrice(p.Stripe.PriceID),
			PortalConfigured:       validStripePortal(p.Stripe.PortalConfigID),
			CatalogConfigured:      catalogOK,
			ReturnOriginConfigured: p.ReturnOrigin != "",
			Reason: saleReason(p.Stripe.NewSalesEnabled, stripeSales, p.Stripe.Configured,
				p.Stripe.MaintenanceEnabled, p.Stripe.Mode == p.Stripe.ExpectedMode,
				p.ReturnOrigin != "", catalogOK, p.StripeIssues),
			Issues: append(append([]string(nil), commonIssues...), p.StripeIssues...),
		},
		PixManual: PaymentChannelPreflight{
			Channel: "asaas_pix_manual", NewSalesRequested: p.Asaas.PixManualNewSalesEnabled,
			NewSalesReady:      p.Asaas.PixManualNewSalesEnabled && manualSales,
			MaintenanceEnabled: p.Asaas.MaintenanceEnabled, MaintenanceReady: asaasMaintenance,
			MaintenanceReason: maintenanceReason(p.Asaas.MaintenanceEnabled, p.Asaas.Configured,
				p.Asaas.Mode == p.Asaas.ExpectedMode, asaasCredentialsReady(p.Asaas)),
			Mode: p.Asaas.Mode, MerchantConfigured: validPaymentUUID(p.Asaas.MerchantID),
			WebhookConfigured: validAsaasWebhook(p.Asaas.WebhookToken),
			CatalogConfigured: catalogOK,
			Reason: saleReason(p.Asaas.PixManualNewSalesEnabled, manualSales, p.Asaas.Configured,
				p.Asaas.MaintenanceEnabled, p.Asaas.Mode == p.Asaas.ExpectedMode,
				true, catalogOK, p.AsaasIssues),
			Issues: append(append([]string(nil), commonIssues...), p.AsaasIssues...),
		},
		PixAutomatic: PaymentChannelPreflight{
			Channel: "asaas_pix_automatic", NewSalesRequested: p.Asaas.PixAutomaticNewSalesEnabled,
			NewSalesReady:      p.Asaas.PixAutomaticNewSalesEnabled && autoSales,
			MaintenanceEnabled: p.Asaas.MaintenanceEnabled, MaintenanceReady: asaasMaintenance,
			MaintenanceReason: maintenanceReason(p.Asaas.MaintenanceEnabled, p.Asaas.Configured,
				p.Asaas.Mode == p.Asaas.ExpectedMode, asaasCredentialsReady(p.Asaas)),
			Mode: p.Asaas.Mode, MerchantConfigured: validPaymentUUID(p.Asaas.MerchantID),
			WebhookConfigured: validAsaasWebhook(p.Asaas.WebhookToken),
			CatalogConfigured: catalogOK,
			Reason: saleReason(p.Asaas.PixAutomaticNewSalesEnabled, autoSales, p.Asaas.Configured,
				p.Asaas.MaintenanceEnabled, p.Asaas.Mode == p.Asaas.ExpectedMode,
				true, catalogOK, p.AsaasIssues),
			Issues: append(append([]string(nil), commonIssues...), p.AsaasIssues...),
		},
	}
}

func maintenanceReason(enabled, configured, modeMatches, credentials bool) string {
	switch {
	case !enabled:
		return PaymentReasonMaintenanceDisabled
	case !configured:
		return PaymentReasonProviderMissing
	case !modeMatches:
		return PaymentReasonModeMismatch
	case !credentials:
		return PaymentReasonMissingCredentials
	default:
		return ""
	}
}

func saleReason(requested, ready, configured, maintenance, modeMatches, returnOrigin, catalog bool, issues []string) string {
	switch {
	case !requested:
		return PaymentReasonDisabledByFlag
	case ready:
		return ""
	case !maintenance:
		return PaymentReasonMaintenanceDisabled
	case !configured:
		return PaymentReasonProviderMissing
	case !modeMatches:
		return PaymentReasonModeMismatch
	case !returnOrigin:
		return PaymentReasonMissingReturnOrigin
	case !catalog:
		return PaymentReasonMissingCatalog
	case len(issues) > 0:
		return PaymentReasonInvalidConfig
	default:
		return PaymentReasonMissingCredentials
	}
}

func (p PaymentsConfig) logValue() slog.Value {
	return slog.GroupValue(
		slog.Group("stripe",
			slog.Bool("configured", p.Stripe.Configured),
			slog.String("mode", p.Stripe.Mode),
			slog.Bool("new_sales", p.Stripe.NewSalesEnabled),
			slog.Bool("maintenance", p.Stripe.MaintenanceEnabled),
			slog.Int("issues", len(p.StripeIssues)),
		),
		slog.Group("asaas",
			slog.Bool("configured", p.Asaas.Configured),
			slog.String("mode", p.Asaas.Mode),
			slog.Bool("pix_manual_new_sales", p.Asaas.PixManualNewSalesEnabled),
			slog.Bool("pix_automatic_new_sales", p.Asaas.PixAutomaticNewSalesEnabled),
			slog.Bool("maintenance", p.Asaas.MaintenanceEnabled),
			slog.Int("issues", len(p.AsaasIssues)),
		),
	)
}

func paymentBool(getenv func(string) (string, bool), key string, fallback bool, issues *[]string) bool {
	raw, present := getenv(key)
	if !present || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		*issues = append(*issues, key+" must be boolean")
		return fallback
	}
	return value
}

func expectedStripeMode(env Environment) string {
	if env == EnvProduction {
		return "live"
	}
	return "test"
}

func expectedAsaasMode(env Environment) string {
	if env == EnvProduction {
		return "production"
	}
	return "test"
}

func validStripeMode(mode string) bool {
	return mode == "test" || mode == "live"
}

func validAsaasMode(mode string) bool {
	return mode == "test" || mode == "production"
}

func validStripeSecret(secret, mode string) bool {
	if !validStripeMode(mode) || len(secret) < 20 {
		return false
	}
	return strings.HasPrefix(secret, "sk_"+mode+"_") || strings.HasPrefix(secret, "rk_"+mode+"_")
}

func validStripeWebhook(secret string) bool {
	return strings.HasPrefix(secret, "whsec_") && len(secret) > len("whsec_")+10
}

func validStripePrice(price string) bool {
	return strings.HasPrefix(price, "price_") && len(price) > len("price_")+3
}

func validStripePortal(id string) bool {
	return strings.HasPrefix(id, "bpc_") && len(id) > len("bpc_")+3
}

func validAsaasAPIKey(key string) bool {
	return validPaymentText(key, 200) && len(key) >= 20
}

func validAsaasWebhook(token string) bool {
	return validPaymentText(token, 255) && len(token) >= 32
}

func validPaymentText(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, r := range value {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}

func validPaymentUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}

func validPaymentOrigin(raw string, env Environment) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") {
		return "", false
	}
	host := u.Hostname()
	loopback := host == "127.0.0.1" || host == "localhost" || host == "::1"
	switch u.Scheme {
	case "https":
	case "http":
		if env != EnvDevelopment || !loopback {
			return "", false
		}
	default:
		return "", false
	}
	if !loopback && !validHostname(host) {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

func stripeCredentialsReady(s StripePaymentConfig) bool {
	return s.Configured && validStripeMode(s.Mode) && s.Mode == s.ExpectedMode &&
		validStripeSecret(s.SecretKey, s.Mode) && validStripeWebhook(s.WebhookSecret) &&
		strings.HasPrefix(s.MerchantID, "acct_")
}

func asaasCredentialsReady(a AsaasPaymentConfig) bool {
	return a.Configured && validAsaasMode(a.Mode) && a.Mode == a.ExpectedMode &&
		validAsaasAPIKey(a.APIKey) && validAsaasWebhook(a.WebhookToken) &&
		a.APIKey != a.WebhookToken && validPaymentUUID(a.MerchantID)
}
