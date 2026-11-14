package enterprisesuite

import (
	"context"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"net/url"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgquota"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type intakeAdmission struct{ budget *orgquota.IntakeBudget }

func (a intakeAdmission) AdmitBatch(ctx context.Context, s app.Source, n int) error {
	if a.budget == nil {
		return app.ErrUnavailable
	}
	return a.budget.CheckBatch(ctx, orgapi.Principal{Actor: orgapi.Actor{AccountID: s.Actor.AccountID, ContributorID: s.Actor.ContributorID, DeviceKeyID: s.Actor.KeyID, Channel: "android"}, Scope: s.Scope, MembershipID: s.MembershipID}, n)
}

// IntakeEvents validates actual source-owned callbacks with the existing queue
// handler. The API does not start a worker: the first real feature must install
// this handler in the existing Dispatcher before activating its producer.
func IntakeEvents(pool *pgxpool.Pool, consumers map[string]eventpg.Effect) (event.Catalog, *eventpg.Handler, error) {
	if len(consumers) == 0 {
		return event.Catalog{}, nil, nil
	}
	names := make([]string, 0, len(consumers))
	for name := range consumers {
		names = append(names, name)
	}
	sort.Strings(names)
	catalog, err := event.NewCatalog(syncpg.EventDefinitions(names))
	if err != nil {
		return event.Catalog{}, nil, err
	}
	handler, err := eventpg.NewHandler(eventpg.NewStore(pool), catalog, consumers)
	if err != nil {
		return event.Catalog{}, nil, err
	}
	return catalog, handler, nil
}

func registerIntake(router chi.Router, o Options, devices *member.DeviceStore, meters *fleetpg.MeterStore, auditor auditing.TransactionRunner, auditCatalog audit.Catalog) error {
	if meters == nil {
		return app.ErrUnavailable
	}
	meterHandler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		return err
	}
	handlers := append([]app.Handler{meterHandler}, o.IntakeHandlers...)
	registry, err := app.NewRegistry(handlers...)
	if err != nil {
		return err
	}
	// Explicit source-owned synthetic policy, never an absent/unbounded budget.
	allowance, err := identity.DefaultQuotaPolicy().Lookup(identity.OperationWrite)
	if err != nil {
		return err
	}
	definition := orgquota.TransportDefinition{Class: "org.sync", Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic immutable meter intake inherits identity v1 write200/hour per organization/account/device, weighted by attempts; production measurement and cleanup remain required."}
	if o.IntakeBudget != nil {
		definition = *o.IntakeBudget
	}
	budget, err := orgquota.NewIntakeBudget(o.Pool, o.CursorSecret, definition)
	if err != nil {
		return err
	}
	noAsync := make(map[app.OperationKey]map[string]app.FollowupMode, len(o.IntakeNoAsyncPlans)+1)
	for key, results := range o.IntakeNoAsyncPlans {
		copied := make(map[string]app.FollowupMode, len(results))
		for result, mode := range results {
			copied[result] = mode
		}
		noAsync[key] = copied
	}
	meterKey := app.OperationKey{Type: fleetpg.MeterReadingOperationType, Schema: 1}
	if _, exists := noAsync[meterKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[meterKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	catalog, _, err := IntakeEvents(o.Pool, o.IntakeConsumers)
	if err != nil {
		return err
	}
	store := syncpg.NewStore(syncpg.Config{Pool: o.Pool, Audit: auditor, AuditCatalog: auditCatalog, EventCatalog: catalog, EventWriter: eventpg.NewStore(o.Pool), Plans: o.IntakePlans, NoAsyncPlans: noAsync,
		ConfirmSource: func(ctx context.Context, tx pgx.Tx, s app.Source) (time.Time, error) {
			_, now, err := devices.CheckCurrentIntakeDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
			if err != nil {
				return time.Time{}, app.SourceDenied(err)
			}
			return now, nil
		},
	})
	origin, err := url.Parse(o.Origin)
	if err != nil {
		return app.ErrUnavailable
	}
	return syncweb.Register(router, syncweb.Options{Enabled: true, Origin: o.Origin, Accounts: o.Accounts, Verifier: auth.Verifier{Pool: o.Pool, Authority: origin.Host, Now: time.Now}, Devices: devices, Service: app.Service{Store: store, Admission: intakeAdmission{budget}, Registry: registry}, Limits: protocol.DefaultLimits()})
}
