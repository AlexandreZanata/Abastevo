package adapters

import (
	"net/http"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters/appstore"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters/play"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

// Fixed subscription management destinations (B-BR-STORE-09).
const (
	appStoreManagementURL = "https://apps.apple.com/account/subscriptions"
	playManagementURL     = "https://play.google.com/store/account/subscriptions"
)

// PlayChannel is a complete Google Play configuration.
type PlayChannel struct {
	PackageName        string
	ServiceAccountJSON []byte
	Products           map[string][]string
	TestAccounts       map[string]struct{}
}

// AppStoreChannel is a complete App Store configuration.
type AppStoreChannel struct {
	BundleID, IssuerID, KeyID string
	PrivateKeyPEM             []byte
	Products                  []string
	SandboxFallback           bool
}

// StoreChannels configures the in-app store channels. Nil channels stay off.
type StoreChannels struct {
	Environment          string
	TokenKey, BindingKey []byte
	Play                 *PlayChannel
	AppStore             *AppStoreChannel
	HTTP                 *http.Client
}

// Storefront is the composed store billing runtime.
type Storefront struct {
	app.Storefront
	// AppStore verifies App Store notifications; nil when the channel is off.
	AppStore *appstore.Client
	// Issues lists channels that failed to start and stayed off.
	Issues []string
}

// NewStorefront builds one verifying service per configured store over the
// shared store. The Play service always exists so the status read and
// export keep working while every channel is off.
func NewStorefront(store *Store, channels StoreChannels, clock func() time.Time) Storefront {
	playPackage := "com.abastevo"
	if channels.Play != nil && channels.Play.PackageName != "" {
		playPackage = channels.Play.PackageName
	}
	environment := channels.Environment
	if environment == "" {
		environment = "production"
	}
	playService := &app.Service{Store: store, Clock: clock, Environment: environment, PackageName: playPackage,
		StoreName: domain.ProviderPlay, ManagementURL: playManagementURL + "?package=" + playPackage}
	result := Storefront{Storefront: app.Storefront{Services: []*app.Service{playService}}}
	tokens, tokenErr := NewTokenCodec(channels.TokenKey)

	if channels.Play != nil {
		account, err := play.NewServiceAccount(channels.Play.ServiceAccountJSON, channels.HTTP)
		var client *play.Client
		if err == nil {
			client, err = play.New(play.Config{PackageName: playPackage, Environment: environment, Plans: channels.Play.Products}, channels.HTTP, account.AccessToken)
		}
		binding, bindErr := NewAccountBinding(channels.BindingKey, environment, playPackage)
		switch {
		case err != nil:
			result.Issues = append(result.Issues, "play: invalid service account or catalog")
		case tokenErr != nil || bindErr != nil:
			result.Issues = append(result.Issues, "play: invalid store keys")
		default:
			playService.Provider, playService.Tokens, playService.AccountBinding = client, tokens, binding
			playService.Catalog = playCatalog(channels.Play.Products)
			playService.PlayTestAccounts = channels.Play.TestAccounts
		}
	}

	if channels.AppStore != nil {
		client, err := appstore.New(appstore.Config{
			BundleID: channels.AppStore.BundleID, Environment: environment, IssuerID: channels.AppStore.IssuerID,
			KeyID: channels.AppStore.KeyID, PrivateKeyPEM: channels.AppStore.PrivateKeyPEM,
			Products: channels.AppStore.Products, SandboxFallback: channels.AppStore.SandboxFallback,
		}, channels.HTTP, nil)
		binding, bindErr := NewAppAccountToken(channels.BindingKey, environment, channels.AppStore.BundleID)
		switch {
		case err != nil:
			result.Issues = append(result.Issues, "app_store: invalid API key or catalog")
		case tokenErr != nil || bindErr != nil:
			result.Issues = append(result.Issues, "app_store: invalid store keys")
		default:
			catalog := make([]app.Product, 0, len(channels.AppStore.Products))
			for _, product := range channels.AppStore.Products {
				catalog = append(catalog, app.Product{ProductID: product, PlanIDs: []string{}})
			}
			result.Services = append(result.Services, &app.Service{
				Store: store, Provider: client, Tokens: tokens, AccountBinding: binding, Clock: clock,
				Environment: environment, PackageName: channels.AppStore.BundleID, StoreName: domain.ProviderAppStore,
				Catalog: catalog, ManagementURL: appStoreManagementURL,
			})
			result.AppStore = client
		}
	}
	return result
}

func playCatalog(products map[string][]string) []app.Product {
	ids := make([]string, 0, len(products))
	for id := range products {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	catalog := make([]app.Product, 0, len(ids))
	for _, id := range ids {
		catalog = append(catalog, app.Product{ProductID: id, PlanIDs: append([]string{}, products[id]...)})
	}
	return catalog
}
