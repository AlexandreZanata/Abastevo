package main

import (
	"net/http"
	"time"

	subscriptionadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

// storeChannels mirrors the API composition: refresh, acknowledgement and
// reconciliation jobs must reach the same configured stores.
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
