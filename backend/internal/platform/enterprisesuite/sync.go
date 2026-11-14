package enterprisesuite

import (
	"context"
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

func registerIntake(router chi.Router, o Options, devices *member.DeviceStore, auditor auditing.TransactionRunner, auditCatalog audit.Catalog) error {
	registry, err := app.NewRegistry(o.IntakeHandlers...)
	if err != nil {
		return err
	}
	var budget *orgquota.IntakeBudget
	if o.IntakeBudget != nil {
		budget, err = orgquota.NewIntakeBudget(o.Pool, o.CursorSecret, *o.IntakeBudget)
		if err != nil {
			return err
		}
	}
	catalog, _, err := IntakeEvents(o.Pool, o.IntakeConsumers)
	if err != nil {
		return err
	}
	store := syncpg.NewStore(syncpg.Config{Pool: o.Pool, Audit: auditor, AuditCatalog: auditCatalog, EventCatalog: catalog, EventWriter: eventpg.NewStore(o.Pool), Plans: o.IntakePlans,
		ConfirmSource: func(ctx context.Context, tx pgx.Tx, s app.Source) (time.Time, error) {
			_, now, err := devices.CheckCurrentDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
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
