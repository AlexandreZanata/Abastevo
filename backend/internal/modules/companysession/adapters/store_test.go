package adapters_test

import (
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"testing"
	"time"
)

func TestUnwiredCompanyPersistenceAndAccountProofFailClosed(t *testing.T) {
	store := &adapters.Store{}
	if err := store.Create(t.Context(), app.Row{}, app.Proof{}, "", time.Now()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(t.Context(), "", time.Now(), true, nil); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := store.Revoke(t.Context(), "", "", time.Now()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := store.PurgeExpired(t.Context(), time.Now(), 500); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := adapters.AccountProof(nil)(t.Context(), "", ""); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
}
