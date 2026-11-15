package enterprisesuite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	fuelapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"net/url"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
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

func registerIntake(router chi.Router, o Options, devices *member.DeviceStore, meters *fleetpg.MeterStore, assets *fleetpg.AssetStore, auditor auditing.TransactionRunner, auditCatalog audit.Catalog) error {
	if meters == nil || assets == nil {
		return app.ErrUnavailable
	}
	meterHandler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		return err
	}
	// EMP-FUEL-002: the fueling owner shares the meter authority's real
	// sources and records its reading through the same fleet store.
	meterAuthority, ok := meters.Authority.(*orgpg.MeterAuthority)
	if !ok || meterAuthority == nil {
		return app.ErrUnavailable
	}
	fuelingHandler, err := fuelpg.NewIntakeHandler((*orgpg.FuelingAuthority)(meterAuthority), meters, &fuelpg.Store{Allocations: assets})
	if err != nil {
		return err
	}
	// EMP-TRIP-002: the trip owners share the same authority sources, fleet
	// readings and allocation port; held facts use the OFFL-008 review items.
	tripStore := &trippg.Store{Allocations: assets, NewID: jobs.NewUUIDv4}
	tripStart, err := trippg.NewStartHandler((*orgpg.TripAuthority)(meterAuthority), meters, assets, tripStore)
	if err != nil {
		return err
	}
	tripFinish, err := trippg.NewFinishHandler((*orgpg.TripAuthority)(meterAuthority), meters, tripStore)
	if err != nil {
		return err
	}
	// EMP-TRIP-003: typed logbook events append to the member's own stored
	// trip; held events use the same OFFL-008 review items.
	tripEvent, err := trippg.NewEventHandler((*orgpg.TripAuthority)(meterAuthority), meters, tripStore, &trippg.EventStore{NewID: jobs.NewUUIDv4})
	if err != nil {
		return err
	}
	// EMP-TRIP-004 (EDR-220): the author's signed submission of a completed trip.
	tripSubmit, err := trippg.NewSubmitHandler((*orgpg.TripAuthority)(meterAuthority), meters, tripStore, tripStore)
	if err != nil {
		return err
	}
	handlers := append([]app.Handler{meterHandler, fuelingHandler, tripStart, tripFinish, tripEvent, tripSubmit}, o.IntakeHandlers...)
	registry, err := app.NewRegistry(handlers...)
	if err != nil {
		return err
	}
	// Explicit source-owned synthetic policy, never an absent/unbounded budget.
	allowance, err := identity.DefaultQuotaPolicy().Lookup(identity.OperationWrite)
	if err != nil {
		return err
	}
	definition := orgquota.TransportDefinition{Class: "org.sync", Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic immutable meter and fueling intake inherits identity v1 write200/hour per organization/account/device, weighted by attempts; production measurement and cleanup remain required."}
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
	noAsync[meterKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup, protocol.AcceptedForReview: app.AtomicReview}
	// EDR-175: no consumer exists until EXP-002 owns the expense projection.
	fuelingKey := app.OperationKey{Type: fuelapp.OperationType, Schema: 1}
	if _, exists := noAsync[fuelingKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[fuelingKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	// EMP-TRIP-002/003 (B-BR-EMP-319, EDR-209): no trip consumer exists yet;
	// held facts are same-transaction review items.
	for _, kind := range []string{tripapp.StartOperationType, tripapp.FinishOperationType, tripapp.EventOperationType} {
		key := app.OperationKey{Type: kind, Schema: 1}
		if _, exists := noAsync[key]; exists {
			return app.ErrUnavailable
		}
		noAsync[key] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup, protocol.AcceptedForReview: app.AtomicReview}
	}
	// EMP-TRIP-004: a submission has no consumer and no held path.
	submitKey := app.OperationKey{Type: tripapp.SubmitOperationType, Schema: 1}
	if _, exists := noAsync[submitKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[submitKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	catalog, _, err := IntakeEvents(o.Pool, o.IntakeConsumers)
	if err != nil {
		return err
	}
	// EMP-OFFL-012: ORG-009's suspension assessment and the capability row
	// decide organization state before every owner.
	plans, ok := devices.Capabilities.(orgpg.IntakePlanStates)
	if !ok || plans == nil {
		return app.ErrUnavailable
	}
	assessor := orgpg.IntakeStateAssessor{Suspensions: &orgpg.SuspensionStore{Roots: orgpg.NewStore(o.Pool)}, Plans: plans}
	store := syncpg.NewStore(syncpg.Config{Pool: o.Pool, Audit: auditor, AuditCatalog: auditCatalog, EventCatalog: catalog, EventWriter: eventpg.NewStore(o.Pool), Plans: o.IntakePlans, NoAsyncPlans: noAsync,
		ConfirmSource: func(ctx context.Context, tx pgx.Tx, s app.Source) (time.Time, error) {
			_, now, err := devices.CheckCurrentIntakeDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
			if errors.Is(err, access.ErrOrgSuspended) {
				// Assessed before any owner; never owner write or read authority.
				_, now, err = devices.CheckSuspendedIntakeDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
			}
			if err != nil {
				return time.Time{}, app.SourceDenied(err)
			}
			return now, nil
		},
		AssessState: assessor.AssessStateTx,
		// EMP-OFFL-009: the intake-only source of one exact ended period.
		ConfirmLate: func(ctx context.Context, tx pgx.Tx, s app.Source) (app.LateAccess, time.Time, error) {
			v, now, err := devices.CheckLateIntakeDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
			if err != nil {
				return app.LateAccess{}, time.Time{}, app.SourceDenied(err)
			}
			return app.LateAccess{End: app.LateEnd(v.End), EndedAt: v.EndedAt}, now, nil
		},
	})
	origin, err := url.Parse(o.Origin)
	if err != nil {
		return app.ErrUnavailable
	}
	verifier := auth.Verifier{Pool: o.Pool, Authority: origin.Host, Now: time.Now}
	if err := syncweb.RegisterReviewResults(router, syncweb.ReviewResultsOptions{Enabled: true, Origin: o.Origin, Accounts: o.Accounts, Verifier: verifier, Devices: devices, Repository: meters.Reviews}); err != nil {
		return err
	}
	// EMP-OFFL-006: the member's purpose-bound reference feed (EDR-216..218).
	roles, ok := meterAuthority.Roles.(syncpg.ReferenceAuthority)
	if !ok || roles == nil {
		return app.ErrUnavailable
	}
	secret := referenceFeedKey(o.CursorSecret)
	reference := &syncpg.ReferenceStore{Pool: o.Pool, Authority: roles, Assets: assets, Secret: secret, NewEpoch: jobs.NewUUIDv4,
		Confirm: func(ctx context.Context, tx pgx.Tx, s app.Source) (time.Time, error) {
			_, now, err := devices.CheckCurrentDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
			if err != nil {
				return time.Time{}, app.SourceDenied(err)
			}
			return now, nil
		},
	}
	if err := syncweb.RegisterReference(router, syncweb.ReferenceOptions{Enabled: true, Origin: o.Origin, Secret: secret, Accounts: o.Accounts, Verifier: verifier, Devices: devices, Repository: reference}); err != nil {
		return err
	}
	return syncweb.Register(router, syncweb.Options{Enabled: true, Origin: o.Origin, Accounts: o.Accounts, Verifier: verifier, Devices: devices, Late: devices, State: devices, Service: app.Service{Store: store, Admission: intakeAdmission{budget}, Registry: registry}, Limits: protocol.DefaultLimits()})
}

// referenceFeedKey separates the reference cursor key from other uses of the
// deployment cursor secret.
func referenceFeedKey(secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("abastevo/reference-feed/v1"))
	return mac.Sum(nil)
}
