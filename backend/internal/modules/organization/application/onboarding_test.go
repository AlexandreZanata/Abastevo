package application_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestOnboardingUnavailablePortsCannotReturnSuccessOrAcceptForgedDecision(t *testing.T) {
	actor := app.CreationActor{AccountID: "7b0e2a54-0000-4000-8000-000000000001", SessionID: "7b0e2a54-0000-4000-8000-000000000002"}
	command, err := d.NewCreation(d.CreationInput{OperationID: "7b0e2a54-0000-4000-8000-000000000003", Profile: d.Profile{Name: "Synthetic company", TimeZone: "UTC", Kinds: []d.Kind{d.Other}}, UnitName: "Main", Terms: d.TermsAcceptance{Version: d.SyntheticTermsVersion, Digest: d.SyntheticTermsDigest, Accepted: true}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		service app.Onboarding
		actor   app.CreationActor
		command d.Creation
		want    error
	}{
		{"off", app.Onboarding{}, actor, command, app.ErrSurfaceDisabled},
		{"missing actor", app.Onboarding{Enabled: true}, app.CreationActor{}, command, app.ErrUnauthenticated},
		{"invalid command", app.Onboarding{Enabled: true}, actor, d.Creation{}, d.ErrInvalid},
		{"missing repository", app.Onboarding{Enabled: true, NewID: jobs.NewUUIDv4}, actor, command, app.ErrUnavailable},
		{"unavailable real adapter", app.Onboarding{Enabled: true, Repository: &adapters.OnboardingStore{}, NewID: jobs.NewUUIDv4}, actor, command, app.ErrUnavailable},
		{"ID provider failure", app.Onboarding{Enabled: true, Repository: &adapters.OnboardingStore{}, NewID: func() (string, error) { return "", errors.New("private random source detail") }}, actor, command, app.ErrUnavailable},
		{"ID collision", app.Onboarding{Enabled: true, Repository: &adapters.OnboardingStore{}, NewID: func() (string, error) { return actor.AccountID, nil }}, actor, command, app.ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, replay, err := test.service.Create(t.Context(), test.actor, test.command)
			if !errors.Is(err, test.want) || replay || got != (app.CreationResult{}) {
				t.Fatal("unsafe refused result", replay, err)
			}
		})
	}
	id, err := kernel.ParseOrgID(actor.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	service := app.Onboarding{Enabled: true, Repository: &adapters.OnboardingStore{}, NewID: jobs.NewUUIDv4}
	got, replay, err := service.Activate(t.Context(), scope, actor, access.Decision{}, command.OperationID(), 1)
	if !errors.Is(err, app.ErrUnauthenticated) || replay || got != (app.ActivationResult{}) {
		t.Fatal("forged empty decision activated setup", err)
	}
	for _, value := range []any{actor, app.Genesis{Scope: scope}, app.CreationResult{OrgID: id}, app.ActivationResult{OrgID: id}} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			if strings.Contains(fmt.Sprintf(verb, value), actor.AccountID) {
				t.Fatal("private onboarding value leaked")
			}
		}
	}
}
