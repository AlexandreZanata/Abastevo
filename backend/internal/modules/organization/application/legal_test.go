package application_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestLegalServiceCannotUseForgedDecisionOrUnavailableProductionPort(t *testing.T) {
	const id = "7b0e2a54-0000-4000-8000-000000000301"
	org, err := kernel.ParseOrgID(id)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	actor := app.CompanyActor{AccountID: "7b0e2a54-0000-4000-8000-000000000001", SessionID: "7b0e2a54-0000-4000-8000-000000000002"}
	declaration, err := d.NewLegalDeclaration("7b0e2a54-0000-4000-8000-000000001001", d.LegalDeclarationInput{Kind: d.Company, LegalName: "Synthetic company", Identifier: "EXEMPL01000170"})
	if err != nil {
		t.Fatal(err)
	}
	command, err := app.DeclareLegalCommand(declaration)
	if err != nil {
		t.Fatal(err)
	}
	source := &adapters.LegalStore{}
	service := app.LegalService{Enabled: true, Repository: source}
	if result, replay, err := service.Write(t.Context(), scope, actor, access.Decision{}, command); !errors.Is(err, app.ErrUnauthenticated) || replay || result != (app.LegalResult{}) {
		t.Fatal("forged write decision accepted", err)
	}
	if result, err := service.Read(t.Context(), scope, actor, access.Decision{}, id, true); !errors.Is(err, app.ErrUnauthenticated) || result != (app.LegalReadResult{}) {
		t.Fatal("forged reveal decision accepted", err)
	}
	if _, err := source.FindLegalEntity(t.Context(), scope, id); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unavailable lookup returned declaration", err)
	}
	if _, err := source.FindEstablishment(t.Context(), scope, id); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unavailable lookup returned establishment", err)
	}
	if result, replay, err := (app.LegalService{}).Write(t.Context(), scope, actor, access.Decision{}, command); !errors.Is(err, app.ErrSurfaceDisabled) || replay || result != (app.LegalResult{}) {
		t.Fatal("default surface exposed", err)
	}
	if err := (app.LegalFilter{Limit: 101}).Validate(); err == nil {
		t.Fatal("unbounded list accepted")
	}
}
func TestLegalCommandsCanonicalizeAndRetainTheirOwnIntent(t *testing.T) {
	const entity = "7b0e2a54-0000-4000-8000-000000000401"
	const operation = "7b0e2a54-0000-4000-8000-000000001001"
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	command, err := app.AddEstablishmentCommand(entity, operation, 1, d.EstablishmentInput{CNPJ: "ex.emp.l01/0001-70", TradeName: " Synthetic trade "})
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := app.AddEstablishmentCommand(entity, operation, 1, d.EstablishmentInput{CNPJ: "EXEMPL01000170", TradeName: "Synthetic trade", Role: d.NotInformed})
	if err != nil || equivalent.Digest(scope) != command.Digest(scope) {
		t.Fatal("equivalent intent changed digest", err)
	}
	copy, err := command.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	copy.Establishment.TradeName = "changed"
	if command.Digest(scope) != equivalent.Digest(scope) {
		t.Fatal("mutable command escaped")
	}
	if _, err := app.WithdrawLegalCommand(entity, operation, 0); err == nil {
		t.Fatal("unversioned withdrawal accepted")
	}
	if _, err := app.ChangeEstablishmentCommand(entity, entity, operation, 1, d.EstablishmentMetadata{}); err == nil {
		t.Fatal("identity collision accepted")
	}
	if _, err := (app.LegalWrite{}).Snapshot(); err == nil {
		t.Fatal("zero command accepted")
	}
}
