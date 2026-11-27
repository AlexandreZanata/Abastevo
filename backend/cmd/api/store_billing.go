package main

import (
	"net/http"
	"time"

	subscriptionadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters/googlepush"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

// storeChannels maps validated configuration to the store channels. A
// channel with configuration issues stays nil, so it stays off.
func storeChannels(cfg config.StoreBillingConfig) subscriptionadapters.StoreChannels {
	channels := subscriptionadapters.StoreChannels{
		Environment: cfg.Environment, TokenKey: cfg.TokenKey, BindingKey: cfg.BindingKey,
		HTTP: &http.Client{Timeout: 5 * time.Second},
	}
	if cfg.Play.Configured {
		channels.Play = &subscriptionadapters.PlayChannel{
			PackageName: cfg.Play.PackageName, ServiceAccountJSON: cfg.Play.ServiceAccountJSON, Products: cfg.Play.Products,
			TestAccounts: cfg.Play.TestAccounts,
		}
	}
	if cfg.AppStore.Configured {
		channels.AppStore = &subscriptionadapters.AppStoreChannel{
			BundleID: cfg.AppStore.BundleID, IssuerID: cfg.AppStore.IssuerID, KeyID: cfg.AppStore.KeyID,
			PrivateKeyPEM: cfg.AppStore.PrivateKeyPEM, Products: cfg.AppStore.Products,
			SandboxFallback: cfg.AppStore.SandboxFallback,
		}
	}
	return channels
}

// playPushVerifier authenticates Pub/Sub push for RTDN, or is nil while the
// push identity is not configured (the route then answers 503).
func playPushVerifier(cfg config.StoreBillingConfig) *googlepush.Verifier {
	if !cfg.Play.Configured || !cfg.Play.PushConfigured {
		return nil
	}
	keys := &googlepush.HTTPKeys{Client: &http.Client{Timeout: 5 * time.Second}}
	return &googlepush.Verifier{Audience: cfg.Play.PushAudience, ServiceAccount: cfg.Play.PushServiceAccount, Keys: keys.Key}
}
