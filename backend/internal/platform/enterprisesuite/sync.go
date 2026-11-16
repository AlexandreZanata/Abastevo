package enterprisesuite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	expensepg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/adapters"
	expenseapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	fuelapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
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

func registerIntake(router chi.Router, o Options, devices *member.DeviceStore, meters *fleetpg.MeterStore, assets *fleetpg.AssetStore, incidents *maintpg.IncidentStore, auditor auditing.TransactionRunner, auditCatalog audit.Catalog) error {
	if meters == nil || assets == nil || incidents == nil {
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
	// EMP-EXP-002 (EDR-287): every accepted fueling stores its one expense in
	// the same transaction; the expense.submit owner records direct expenses
	// with the same authority sources, asset locks and allocation port.
	expenses := &expensepg.Store{Allocations: assets, NewID: jobs.NewUUIDv4}
	fuelingHandler, err := fuelpg.NewIntakeHandler((*orgpg.FuelingAuthority)(meterAuthority), meters, &fuelpg.Store{Allocations: assets}, expenses)
	if err != nil {
		return err
	}
	expenseHandler, err := expensepg.NewIntakeHandler((*orgpg.ExpenseAuthority)(meterAuthority), meters, expenses)
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
	// EMP-CHECK-003 (EDR-240): reports made inside a trip are timeline facts.
	tripEvent, err := trippg.NewEventHandler((*orgpg.TripAuthority)(meterAuthority), meters, tripStore, &trippg.EventStore{NewID: jobs.NewUUIDv4, Reports: incidents})
	if err != nil {
		return err
	}
	// EMP-TRIP-004 (EDR-220): the author's signed submission of a completed trip.
	tripSubmit, err := trippg.NewSubmitHandler((*orgpg.TripAuthority)(meterAuthority), meters, tripStore, tripStore)
	if err != nil {
		return err
	}
	// EMP-TRIP-008 (EDR-229): the assignee's mission response, decided on
	// current facts at receipt; no held fact and no late holder.
	missionResponse, err := trippg.NewMissionResponseHandler((*orgpg.TripAuthority)(meterAuthority), &trippg.MissionStore{})
	if err != nil {
		return err
	}
	// EMP-CHECK-003 (EDR-238): the reporter's signed damage or incident report
	// shares the trip owners' authority sources, locks, allocation and trips.
	report, err := maintpg.NewReportHandler((*orgpg.ReportAuthority)(meterAuthority), meters, assets, tripStore, incidents)
	if err != nil {
		return err
	}
	// EMP-CHECK-002 (EDR-284): the author's signed checklist run, rechecked at
	// receipt against the server's template and policy history.
	run, err := maintpg.NewRunHandler((*orgpg.RunAuthority)(meterAuthority), meters, assets, &maintpg.RunStore{})
	if err != nil {
		return err
	}
	handlers := []app.Handler{meterHandler, fuelingHandler, expenseHandler, tripStart, tripFinish, tripEvent, tripSubmit, missionResponse, report, run}
	// EMP-TRIP-011 (EDR-253): pause, resume and cancellation of the member's
	// own trip at the expected revision; held commands use the same OFFL-008
	// review items.
	for _, kind := range tripapp.LifecycleOperationTypes {
		lifecycle, err := trippg.NewTransitionHandler(kind, (*orgpg.TripAuthority)(meterAuthority), meters, tripStore, tripStore)
		if err != nil {
			return err
		}
		handlers = append(handlers, lifecycle)
	}
	// EMP-GPS-004 (EDR-277/278): the tracking session and batch owners share
	// the trip authority and the stored trips; no held form, no follow-up.
	trackingStore := &trackingpg.Store{}
	tracking, err := trackingpg.NewSessionHandlers((*orgpg.TripAuthority)(meterAuthority), tripStore, trackingStore)
	if err != nil {
		return err
	}
	trackingBatch, err := trackingpg.NewBatchHandler((*orgpg.TripAuthority)(meterAuthority), tripStore, trackingStore)
	if err != nil {
		return err
	}
	handlers = append(append(append(handlers, tracking...), trackingBatch), o.IntakeHandlers...)
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
	// EDR-287: the fueling's expense is projected atomically, never by an
	// asynchronous consumer.
	fuelingKey := app.OperationKey{Type: fuelapp.OperationType, Schema: 1}
	if _, exists := noAsync[fuelingKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[fuelingKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	// EMP-EXP-002 (EDR-289): a direct expense has no consumer and no held path.
	expenseKey := app.OperationKey{Type: expenseapp.OperationType, Schema: 1}
	if _, exists := noAsync[expenseKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[expenseKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	// EMP-TRIP-002/003/011 (B-BR-EMP-319, EDR-209, EDR-253): no trip consumer
	// exists yet; held facts are same-transaction review items.
	for _, kind := range append([]string{tripapp.StartOperationType, tripapp.FinishOperationType, tripapp.EventOperationType}, tripapp.LifecycleOperationTypes...) {
		key := app.OperationKey{Type: kind, Schema: 1}
		if _, exists := noAsync[key]; exists {
			return app.ErrUnavailable
		}
		noAsync[key] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup, protocol.AcceptedForReview: app.AtomicReview}
	}
	// EMP-TRIP-004/015: a submission has no consumer; a stale or duplicate
	// one is a same-transaction held review item (EDR-235).
	submitKey := app.OperationKey{Type: tripapp.SubmitOperationType, Schema: 1}
	if _, exists := noAsync[submitKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[submitKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup, protocol.AcceptedForReview: app.AtomicReview}
	// EMP-TRIP-008 (EDR-229): no mission consumer exists; nothing is held.
	missionKey := app.OperationKey{Type: tripapp.MissionRespondOperationType, Schema: 1}
	if _, exists := noAsync[missionKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[missionKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	// EMP-CHECK-003: a report has no consumer and no held path.
	reportKey := app.OperationKey{Type: maintapp.ReportOperationType, Schema: 1}
	if _, exists := noAsync[reportKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[reportKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	// EMP-CHECK-002: a run has no consumer and no held path (EDR-284).
	runKey := app.OperationKey{Type: maintapp.RunOperationType, Schema: 1}
	if _, exists := noAsync[runKey]; exists {
		return app.ErrUnavailable
	}
	noAsync[runKey] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	// EMP-GPS-004: tracking operations have no consumer and no held path.
	for _, kind := range append(trackingapp.SessionOperationTypes(), trackingapp.BatchOperationType) {
		key := app.OperationKey{Type: kind, Schema: 1}
		if _, exists := noAsync[key]; exists {
			return app.ErrUnavailable
		}
		noAsync[key] = map[string]app.FollowupMode{protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}
	}
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
