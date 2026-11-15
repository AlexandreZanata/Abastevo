// Package enterprisesuite composes the private enterprise entries. Runtime
// credentials/grants, operator flags and legal approval remain independent.
package enterprisesuite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"time"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	companyweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	documentpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	documentweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters/http"
	documentapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	documentdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fleetweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters/http"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meterdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	proofauth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters/http"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	memberweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters/http"
	memberapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	accessweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	orgweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	exportapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	notifypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/adapters"
	notifyweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/adapters/http"
	notifyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/application"
	notify "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	reviewpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/adapters"
	reviewweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/adapters/http"
	reviewapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters/http"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgquota"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Enabled      bool
	Origin       string
	CursorSecret []byte
	Pool         *pgxpool.Pool
	Accounts     *account.Service
	DeviceLimit  int64
	// The synthetic runtime owns the immutable meter producer and its explicit
	// transport allowance. Additional owners supply their actual follow-up plans;
	// production measurement and Android producer activation remain separate.
	IntakeHandlers     []syncapp.Handler
	IntakeBudget       *orgquota.TransportDefinition
	IntakePlans        map[string]map[string][]string
	IntakeNoAsyncPlans map[syncapp.OperationKey]map[string]syncapp.FollowupMode
	IntakeConsumers    map[string]eventpg.Effect
	// NoticeKinds are source-owned notification kinds registered by actual
	// producers. None exists yet; the own inbox routes serve an empty catalog
	// and the notif.dispatch consumer is composed with the first producer.
	NoticeKinds []notify.Kind
	// DocumentStorage is the private organization bucket for synthetic
	// document originals. Nil keeps the routes mounted while bytes and content
	// are unavailable; provider qualification remains a separate gate.
	DocumentStorage *mediapg.StorageConfig
	// ExportStorage is the private bucket for export files; nil keeps every
	// export transfer unavailable (EMP-RPT-001).
	ExportStorage *exportpg.StorageConfig
	// ExportFormats are additional admitted export formats. The suite always
	// composes the operational list kinds of EMP-RPT-002; tests may add
	// synthetic kinds.
	ExportFormats []exportapp.Format
	// MeterPlausibilityOff stores field readings VALID without flags (the
	// EMP-METER-003 rollback, EDR-185). The zero value validates.
	MeterPlausibilityOff bool
}

// Register performs no I/O or provisioning. The default mounts no private
// entry; enabled commands still require an ordinary role and current proof.
func Register(router chi.Router, o Options) error {
	if !o.Enabled {
		return nil
	}
	deviceLimit := o.DeviceLimit
	if deviceLimit == 0 {
		deviceLimit = 3
	}
	if deviceLimit < 1 || deviceLimit > 5 {
		return memberapp.ErrRegisterUnavailable
	}
	companyService := &company.Service{Enabled: true, Store: &companypg.Store{Pool: o.Pool}, Hasher: accountdomain.SHA256Hasher{}, Prove: companypg.AccountProof(o.Accounts), Now: time.Now, NewID: jobs.NewUUIDv4, NewSecret: accountdomain.GenerateToken}
	cookies, err := companyweb.New(companyService, o.Origin)
	if err != nil {
		return err
	}
	definitions := append(orgpg.OnboardingAuditDefinitions(), cappg.AuditDefinitions()...)
	definitions = append(definitions, orgpg.LegalAuditDefinitions()...)
	definitions = append(definitions, orgpg.UnitAuditDefinitions()...)
	definitions = append(definitions, fleetpg.AssetAuditDefinitions()...)
	definitions = append(definitions, fleetpg.MeterAuditDefinitions()...)
	definitions = append(definitions, fleetpg.MeterReviewAuditDefinitions()...)
	definitions = append(definitions, orgpg.CostCentreAuditDefinitions()...)
	definitions = append(definitions, orgpg.TeamAuditDefinitions()...)
	definitions = append(definitions, orgpg.SettingsAuditDefinitions()...)
	definitions = append(definitions, member.EmployeeAuditDefinitions()...)
	definitions = append(definitions, member.InvitationAuditDefinitions()...)
	definitions = append(definitions, member.RedemptionAuditDefinitions()...)
	definitions = append(definitions, member.MembershipAccessAuditDefinitions()...)
	definitions = append(definitions, member.OffboardingAuditDefinitions()...)
	definitions = append(definitions, member.RehireAuditDefinitions()...)
	definitions = append(definitions, member.EmployeeMergeAuditDefinitions()...)
	definitions = append(definitions, member.DeviceAuditDefinitions()...)
	definitions = append(definitions, accesspg.RoleAuditDefinitions()...)
	definitions = append(definitions, orgpg.OwnershipTransferAuditDefinitions()...)
	definitions = append(definitions, syncpg.AuditDefinitions()...)
	definitions = append(definitions, reviewpg.PeriodAuditDefinitions()...)
	definitions = append(definitions, maintpg.ChecklistAuditDefinitions()...)
	definitions = append(definitions, trippg.ReviewAuditDefinitions()...)
	definitions = append(definitions, documentpg.AuditDefinitions()...)
	definitions = append(definitions, mediapg.AuditDefinitions()...)
	definitions = append(definitions, exportpg.AuditDefinitions()...)
	auditCatalog, err := audit.NewCatalog(definitions)
	if err != nil {
		return err
	}
	auditStore := auditpg.NewStore(o.Pool, auditCatalog)
	capabilities, err := cap.NewCatalog([]string{"organizations"})
	if err != nil {
		return err
	}
	flags := cappg.NewStore(o.Pool, capabilities, auditStore)
	roots := orgpg.NewStore(o.Pool)
	roles := accesspg.NewStore(o.Pool)
	memberStore := member.NewStore(o.Pool)
	store := &orgpg.OnboardingStore{Pool: o.Pool, Roots: roots, Memberships: memberStore, Roles: roles, Audit: auditStore, Capabilities: flags}
	allowance, err := identity.DefaultQuotaPolicy().Lookup(identity.OperationWrite)
	if err != nil {
		return err
	}
	admission, err := orgquota.New(o.Pool, o.CursorSecret, []orgquota.Definition{
		{Class: "org.setup", Permissions: []string{"org.settings.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic setup/settings only: share identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
		{Class: "org.legal", Permissions: []string{"org.legal_entity.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic declarations only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
		{Class: "org.structure", Permissions: []string{"org.structure.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic structure only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
		{Class: "fleet.asset", Permissions: []string{"fleet.asset.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic fleet configuration inherits identity v1 write 200/hour on each current organization/account/web-or-device axis; production measurement and cleanup remain required."},
		{Class: "fleet.assignment", Permissions: []string{"fleet.asset.assign"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic driver assignments inherit identity v1 write 200/hour on each current organization/account/web axis (a command names at most 100 employees); production measurement and cleanup remain required."},
		{Class: "fleet.reservation", Permissions: []string{"fleet.reservation.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic reservations and blocks inherit identity v1 write 200/hour on each current organization/account/web axis; production measurement and cleanup remain required."},
		{Class: "check.template", Permissions: []string{"check.template.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic checklist template and policy commands inherit identity v1 write 200/hour on each current organization/account/web axis; production measurement and cleanup remain required."},
		{Class: "meter.review", Permissions: []string{"meter.reading.correct"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic held meter review decisions inherit identity v1 write200/hour on every current organization/account/web-or-device axis; production measurement and cleanup remain required."},
		{Class: "idn.invitation", Permissions: []string{"idn.invitation.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic invitation writes use the existing identity v1 write allowance on every axis; production measurement remains required."},
		{Class: "idn.membership", Permissions: []string{"idn.membership.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic membership lifecycle uses identity v1 write200/hour on every axis; production measurement remains required."},
		{Class: "idn.employee", Permissions: []string{"idn.employee.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic employee register only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
		{Class: "idn.device", Permissions: []string{"idn.credential.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic scoped device enrollment and revocation inherit identity v1 write200/hour on every axis; current source membership precedes admission and production measurement remains required."},
		{Class: "auth.role", Permissions: []string{"auth.role.assign"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic role management only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
		{Class: "trip.review", Permissions: []string{"trip.trip.review"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic trip review decisions inherit identity v1 write 200/hour on every organization/account/web axis; production measurement and cleanup remain required."},
		{Class: "fin.period", Permissions: []string{"fin.period.close", "fin.period.reopen"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic period lock, reopen and re-lock commands inherit identity v1 write 200/hour on every organization/account/web axis; production measurement and cleanup remain required."},
		{Class: "rpt.export", Permissions: []string{"rpt.export.run"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic export requests inherit identity v1 write 200/hour on every organization/account/web axis besides the three-open-job limit; production measurement and cleanup remain required."},
		{Class: "org.ownership_transfer", Permissions: []string{"org.ownership.transfer", "org.ownership.respond"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic ownership transfer only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
		{Class: "doc.document", Permissions: []string{"doc.document.attach"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic document upload sessions inherit identity v1 write 200/hour on every axis; byte transfers keep the orgmedia slot limits and production measurement remains required."},
	})
	if err != nil {
		return err
	}
	gateway := orgapi.Gateway{Enabled: true, Origin: o.Origin, CursorSecret: o.CursorSecret, Now: time.Now, WriteAdmission: admission}
	authorization := access.Authorization{Source: roles, Capabilities: accesspg.PilotEntitlements{Enabled: true, Catalog: capabilities, States: flags}, Records: orgweb.SetupRecords{}, Now: time.Now}
	if err := orgweb.Register(router, org.Onboarding{Enabled: true, Repository: store, NewID: jobs.NewUUIDv4}, cookies, gateway, authorization, roots, creationAdmission(o)); err != nil {
		return err
	}
	if err := orgweb.RegisterCreatedSetups(router, true, store, cookies); err != nil {
		return err
	}
	legal := &orgpg.LegalStore{Pool: o.Pool, Memberships: store.Memberships, Roles: roles, Audit: auditStore, Capabilities: flags}
	legalAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: orgweb.LegalRecords{}, Now: time.Now}
	if err := orgweb.RegisterLegal(router, org.LegalService{Enabled: true, Repository: legal, NewID: jobs.NewUUIDv4}, cookies, gateway, legalAuthorization, roots, legal); err != nil {
		return err
	}
	units := &orgpg.UnitStore{Pool: o.Pool, Memberships: store.Memberships, Roles: roles, Audit: auditStore, Capabilities: flags}
	unitAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: orgweb.UnitRecords{}, Now: time.Now}
	if err := orgweb.RegisterUnits(router, org.UnitService{Enabled: true, Repository: units, NewID: jobs.NewUUIDv4}, cookies, gateway, unitAuthorization, roots, units); err != nil {
		return err
	}
	fleetAuthority := &orgpg.FleetAuthority{Memberships: store.Memberships, Roles: roles, Capabilities: flags}
	// EMP-TRIP-002 (EDR-206): an IN_PROGRESS trip is BR-FLEET-010 open work;
	// work orders join this source when the maintenance epic delivers them.
	// Trips are the FLEET-007 active-use source (EDR-206); NoMaintenanceHolds
	// stays the explicit "no source" answer until maintenance holds exist.
	assets := &fleetpg.AssetStore{Pool: o.Pool, Authority: fleetAuthority, Audit: auditStore, NewID: jobs.NewUUIDv4, OpenWork: trippg.OpenWork{}, ActiveUse: trippg.ActiveUse{}, MaintenanceHolds: fleetpg.NoMaintenanceHolds{}}
	// EMP-AUTH-006: structural grants on an asset resolve its current
	// allocation through the FLEET-005 port, in Prepare and in the transaction.
	assetStructure := &orgpg.ResourceStructure{Kind: orgpg.AssetStructure, Pool: o.Pool, Allocations: assets}
	fleetAuthority.Structure = assetStructure
	// EMP-FLEET-006: SELF grants cover the assets assigned to the member,
	// through the same Fleet port in Prepare and in the transaction.
	assetAssignee := &orgpg.ResourceAssignee{Pool: o.Pool, Assignments: assets}
	fleetAuthority.Assignee = assetAssignee
	assetAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: fleetweb.FleetRecords{}, Structure: assetStructure, Assignee: assetAssignee, Now: time.Now}
	if err := fleetweb.RegisterAssets(router, fleetapp.AssetService{Enabled: true, Repository: assets, NewID: jobs.NewUUIDv4}, cookies, gateway, assetAuthorization); err != nil {
		return err
	}
	if err := fleetweb.RegisterAssetLifecycle(router, fleetapp.AssetLifecycleService{Enabled: true, Repository: assets}, cookies, gateway, assetAuthorization); err != nil {
		return err
	}
	// EMP-FLEET-005 history reads; the same store is the snapshot port that
	// record-creating modules call (no producer is wired before them).
	if err := fleetweb.RegisterAssetAllocations(router, fleetapp.AllocationHistoryService{Enabled: true, Repository: assets}, cookies, gateway, assetAuthorization); err != nil {
		return err
	}
	// EMP-FLEET-006 assignment commands and reads on the same store.
	if err := fleetweb.RegisterAssetAssignments(router, fleetapp.AssignmentService{Enabled: true, Repository: assets, NewID: jobs.NewUUIDv4}, cookies, gateway, assetAuthorization); err != nil {
		return err
	}
	// EMP-FLEET-011 implement attachments under fresh fleet.asset.manage.
	if err := fleetweb.RegisterAssetAttachments(router, fleetapp.AttachmentService{Enabled: true, Repository: assets, NewID: jobs.NewUUIDv4}, cookies, gateway, assetAuthorization); err != nil {
		return err
	}
	// EMP-FLEET-007 reservations, blocks and the availability read.
	if err := fleetweb.RegisterAssetReservations(router, fleetapp.ReservationService{Enabled: true, Repository: assets, NewID: jobs.NewUUIDv4}, cookies, gateway, assetAuthorization); err != nil {
		return err
	}
	// EMP-CHECK-001 checklist templates (organization target) and the
	// applicable template of an asset (fleet.asset.read on the asset).
	checklists := &maintpg.ChecklistStore{Pool: o.Pool, Authority: &orgpg.ChecklistAuthority{Memberships: store.Memberships, Roles: roles, Capabilities: flags, Structure: assetStructure, Assignee: assetAssignee}, Audit: auditStore}
	templateAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: maintweb.ChecklistRecords{}, Now: time.Now}
	applicableAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: maintweb.ChecklistRecords{}, Structure: assetStructure, Assignee: assetAssignee, Now: time.Now}
	if err := maintweb.RegisterChecklists(router, maintapp.ChecklistService{Enabled: true, Repository: checklists, NewID: jobs.NewUUIDv4}, cookies, gateway, templateAuthorization, applicableAuthorization); err != nil {
		return err
	}
	centres := &orgpg.CostCentreStore{Pool: o.Pool, Memberships: store.Memberships, Roles: roles, Audit: auditStore, Capabilities: flags}
	centreAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: orgweb.StructureRecords{}, Now: time.Now}
	if err := orgweb.RegisterCostCentres(router, org.CostCentreService{Enabled: true, Repository: centres, NewID: jobs.NewUUIDv4}, cookies, gateway, centreAuthorization, roots, centres); err != nil {
		return err
	}
	teams := &orgpg.TeamStore{Pool: o.Pool, Memberships: store.Memberships, Roles: roles, Audit: auditStore, Capabilities: flags}
	if err := orgweb.RegisterTeams(router, org.TeamService{Enabled: true, Repository: teams, NewID: jobs.NewUUIDv4}, cookies, gateway, centreAuthorization, roots, teams); err != nil {
		return err
	}

	settings := &orgpg.SettingsStore{Pool: o.Pool, Memberships: store.Memberships, Roles: roles, Audit: auditStore, Capabilities: flags}
	contexts := &orgpg.AccountContextStore{Pool: o.Pool, Memberships: store.Memberships, Roles: roles, Capabilities: flags, Settings: settings}
	if err := orgweb.RegisterAccountContexts(router, true, contexts, cookies); err != nil {
		return err
	}
	settingsAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: orgweb.SettingsRecords{}, Now: time.Now}
	if err := orgweb.RegisterSettings(router, org.SettingsService{Enabled: true, Repository: settings}, cookies, gateway, settingsAuthorization, roots); err != nil {
		return err
	}

	employees := &member.EmployeeStore{Pool: o.Pool, Authority: &orgpg.EmployeeAuthority{Memberships: store.Memberships, Roles: roles, Capabilities: flags}, Audit: auditStore}
	employeeAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: memberweb.EmployeeRecords{}, Now: time.Now}
	if err := memberweb.RegisterEmployees(router, memberapp.EmployeeService{Enabled: true, Repository: employees, NewID: jobs.NewUUIDv4}, cookies, gateway, employeeAuthorization, roots, employees); err != nil {
		return err
	}

	secretFactory, err := member.InvitationSecretFactory(o.CursorSecret)
	if err != nil {
		return err
	}
	invitations := &member.InvitationStore{Pool: o.Pool, Memberships: store.Memberships, Authority: &orgpg.InvitationAuthority{Memberships: store.Memberships, Roles: roles, Capabilities: flags, Settings: settings}, Audit: auditStore}
	invitationAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: memberweb.InvitationRecords{}, Now: time.Now}
	if err := memberweb.RegisterInvitations(router, memberapp.InvitationService{Enabled: true, Repository: invitations, NewID: jobs.NewUUIDv4, NewSecret: secretFactory}, cookies, gateway, invitationAuthorization, roots); err != nil {
		return err
	}

	redemption := &member.RedemptionStore{Pool: o.Pool, Authority: &orgpg.RedemptionAuthority{Roles: roles, Capabilities: flags, Settings: settings}, Managers: invitations.Authority, Memberships: memberStore, Roles: roles, Audit: auditStore, Secret: append([]byte(nil), o.CursorSecret...), NewID: jobs.NewUUIDv4}
	if e := memberweb.RegisterRedemption(router, true, redemption, cookies, gateway, invitationAuthorization, roots, unknownInvitationAdmission(o)); e != nil {
		return e
	}

	membershipAccess := &member.MembershipAccessStore{Pool: o.Pool, Authority: &orgpg.MembershipAccessAuthority{Memberships: memberStore, Roles: roles, Capabilities: flags}, Memberships: memberStore, Periods: roles, Audit: auditStore}
	membershipAccessAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: memberweb.MembershipAccessRecords{}, Now: time.Now}
	if e := memberweb.RegisterMembershipAccess(router, memberapp.MembershipAccessService{Enabled: true, Repository: membershipAccess}, cookies, gateway, membershipAccessAuthorization, roots); e != nil {
		return e
	}

	offboarding := &member.OffboardingStore{Pool: o.Pool, Authority: &orgpg.OffboardingAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: memberStore, Roles: roles, Capabilities: flags}}, Memberships: memberStore, Audit: auditStore}
	if e := memberweb.RegisterOffboarding(router, memberapp.OffboardingService{Enabled: true, Repository: offboarding}, cookies, gateway, membershipAccessAuthorization, roots); e != nil {
		return e
	}

	rehire := &member.RehireStore{Pool: o.Pool, Employees: memberStore, Authority: &orgpg.RehireAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: memberStore, Roles: roles, Capabilities: flags}}, Audit: auditStore}
	if e := memberweb.RegisterRehire(router, memberapp.RehireService{Enabled: true, Repository: rehire}, cookies, gateway, membershipAccessAuthorization, roots); e != nil {
		return e
	}

	employeeMerge := &member.EmployeeMergeStore{Pool: o.Pool, Authority: &orgpg.EmployeeMergeAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: memberStore, Roles: roles, Capabilities: flags}}, Teams: orgpg.EmployeeMergeTeamProjector{NewID: jobs.NewUUIDv4}, Audit: auditStore}
	mergeAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: memberweb.EmployeeMergeRecords{}, Now: time.Now}
	if e := memberweb.RegisterEmployeeMerge(router, memberapp.EmployeeMergeService{Enabled: true, Repository: employeeMerge}, cookies, gateway, mergeAuthorization, roots); e != nil {
		return e
	}
	deviceStore := &member.DeviceStore{Pool: o.Pool, Authority: &orgpg.DeviceAuthority{OffboardingAuthority: orgpg.OffboardingAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: memberStore, Roles: roles, Capabilities: flags}}}, Audit: auditStore, Capabilities: flags, Limit: deviceLimit, NewID: jobs.NewUUIDv4}
	deviceAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: memberweb.DeviceRecords{}, Now: time.Now}
	origin, e := url.Parse(o.Origin)
	if e != nil {
		return memberapp.ErrRegisterUnavailable
	}
	if e := memberweb.RegisterDevices(router, true, deviceStore, cookies, gateway, deviceAuthorization, roots, o.Accounts, proofauth.Verifier{Pool: o.Pool, Authority: origin.Host, Now: time.Now}); e != nil {
		return e
	}
	meterAuthority := &orgpg.MeterAuthority{Memberships: memberStore, Roles: roles, Capabilities: flags, Devices: deviceStore, Structure: assetStructure, Assignee: assetAssignee}
	// Held readings resolve their stored asset/unit snapshot (07-permissions example 3).
	heldStructure := &orgpg.ResourceStructure{Kind: orgpg.HeldMeterReadingStructure, Pool: o.Pool}
	var plausibility *meterdomain.Plausibility
	if !o.MeterPlausibilityOff {
		policy := meterdomain.DefaultPlausibility()
		plausibility = &policy
	}
	reviews := &fleetpg.MeterReviewStore{Pool: o.Pool, Plausibility: plausibility, Authority: &orgpg.MeterReviewAuthority{Memberships: memberStore, Roles: roles, Capabilities: flags, Structure: heldStructure}, Audit: auditStore, NewID: jobs.NewUUIDv4,
		ConfirmSource: func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (time.Time, error) {
			_, now, err := deviceStore.CheckCurrentDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
			return now, err
		},
	}
	// EMP-METER-004: the fueling and trip stores answer the approved-parent
	// port (EMP-TRIP-015, B-BR-EMP-368); no recalculation consumer exists, so
	// nothing is published (EDR-189).
	meters := &fleetpg.MeterStore{Pool: o.Pool, Authority: meterAuthority, Audit: auditStore, Reviews: reviews, Plausibility: plausibility,
		Parents: fleetpg.MeterParentSet{&fuelpg.Store{Allocations: assets}, &trippg.Store{}}}
	if err := fleetweb.RegisterMeters(router, fleetapp.MeterService{Enabled: true, Repository: meters, NewID: jobs.NewUUIDv4}, cookies, gateway, assetAuthorization); err != nil {
		return err
	}
	if err := fleetweb.RegisterMeterCorrections(router, fleetapp.MeterCorrectionService{Enabled: true, Repository: meters, NewID: jobs.NewUUIDv4}, cookies, gateway, assetAuthorization); err != nil {
		return err
	}
	reviewAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: fleetweb.MeterReviewRecords{}, Structure: heldStructure, Now: time.Now}
	if err := fleetweb.RegisterMeterReviews(router, fleetapp.MeterReviewService{Enabled: true, Repository: reviews}, cookies, gateway, reviewAuthorization); err != nil {
		return err
	}
	if err := registerIntake(router, o, deviceStore, meters, assets, auditStore, auditCatalog); err != nil {
		return err
	}

	periods := &reviewpg.PeriodStore{Pool: o.Pool, Authority: &orgpg.PeriodAuthority{Memberships: store.Memberships, Roles: roles, Capabilities: flags}, Audit: auditStore}
	periodAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: reviewweb.PeriodRecords{}, Now: time.Now}
	if err := reviewweb.RegisterPeriods(router, reviewapp.PeriodService{Enabled: true, Repository: periods, NewID: jobs.NewUUIDv4}, cookies, gateway, periodAuthorization); err != nil {
		return err
	}

	// EMP-TRIP-004 (EDR-222): the reviewer read and decision on a stored
	// trip, structural grants through the trip's stored snapshot.
	tripStructure := &orgpg.ResourceStructure{Kind: orgpg.TripStructure, Pool: o.Pool}
	// EMP-TRIP-016 (EDR-232): decisions read the period of the stored start
	// reporting day through the APPR-006 gate.
	// EMP-TRIP-015 (EDR-236/237): the same authority proves amendments and the
	// meter store supersedes an approved amendment's readings.
	tripReviewAuthority := &orgpg.TripReviewAuthority{Memberships: memberStore, Roles: roles, Capabilities: flags, Structure: tripStructure}
	tripReviews := &trippg.ReviewStore{Pool: o.Pool, Authority: tripReviewAuthority, Audit: auditStore, Periods: reviewpg.PeriodGate{}, Amenders: tripReviewAuthority, Readings: meters}
	tripReviewAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: tripweb.ReviewRecords{}, Structure: tripStructure, Now: time.Now}
	if err := tripweb.RegisterReview(router, tripapp.ReviewService{Enabled: true, Repository: tripReviews, NewID: jobs.NewUUIDv4}, cookies, gateway, tripReviewAuthorization); err != nil {
		return err
	}

	exportAuthority := &orgpg.ExportAuthority{Memberships: store.Memberships, Roles: roles, Capabilities: flags}
	if err := registerExports(router, o, cookies, gateway, exportAuthority, authorization.Capabilities, roles, auditStore); err != nil {
		return err
	}

	roleStore := accesspg.NewRoleStore(o.Pool, roles, auditStore, &orgpg.RoleAuthority{Memberships: store.Memberships, Roles: roles, Capabilities: flags})
	roleAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: accessweb.RoleRecords{}, Now: time.Now}
	if err := accessweb.RegisterRoles(router, access.RoleService{Enabled: true, Repository: roleStore, NewID: jobs.NewUUIDv4}, cookies, gateway, roleAuthorization, orgRootResolver{roots}, roles); err != nil {
		return err
	}

	transferStore := orgpg.NewOwnershipTransferStore(o.Pool, roles, auditStore)
	transferStore.Memberships, transferStore.Capabilities = store.Memberships, flags
	transferAuthorization := access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: orgweb.OwnershipTransferRecords{}, Now: time.Now}
	if err := orgweb.RegisterOwnershipTransfer(router, org.OwnershipTransferService{Enabled: true, Repository: transferStore, NewID: jobs.NewUUIDv4, Now: time.Now}, cookies, gateway, transferAuthorization, roots, memberStore); err != nil {
		return err
	}

	if err := registerNotices(router, o, cookies, gateway, roles, allowance); err != nil {
		return err
	}
	if err := registerDocuments(router, o, cookies, gateway, access.Authorization{Source: roles, Capabilities: authorization.Capabilities, Records: documentweb.DocumentRecords{}, Now: time.Now}, roles, auditStore); err != nil {
		return err
	}

	cookies.RegisterRoutes(router)
	return nil
}

// registerNotices mounts the four own-inbox routes (EMP-NOTIF-001). Their
// writes use a dedicated route-class admission, not a catalog permission.
func registerNotices(router chi.Router, o Options, cookies companyweb.Handler, gateway orgapi.Gateway, roles *accesspg.Store, allowance identity.OperationQuota) error {
	kinds, err := notify.NewCatalog(o.NoticeKinds)
	if err != nil {
		return err
	}
	admission, err := orgquota.New(o.Pool, o.CursorSecret, []orgquota.Definition{
		{Class: "notif.self", Permissions: []string{notifyweb.WriteClass}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic own read marks and notice preferences inherit identity v1 write200/hour on every axis after current own-membership proof; production measurement remains required."},
	})
	if err != nil {
		return err
	}
	store := &notifypg.Store{Pool: o.Pool, Catalog: kinds, Authority: roles, NewID: jobs.NewUUIDv4}
	return notifyweb.Register(router, notifyweb.Options{Enabled: true, Service: notifyapp.Service{Enabled: true, Catalog: kinds, Repository: store}, Cookies: cookies, Gateway: gateway, Admission: admission})
}

// registerDocuments composes EMP-DOC-002 over the EMP-PLAT-006 private
// original service and the EMP-PRIV-005 access log. Without configured
// storage, declarations work while bytes and content stay unavailable.
func registerDocuments(router chi.Router, o Options, cookies companyweb.Handler, gateway orgapi.Gateway, authorization access.Authorization, roles *accesspg.Store, auditStore *auditpg.Store) error {
	authority := documentpg.CurrentAuthority{Source: roles, Now: time.Now}
	var objects documentObjects = documentpg.UnavailableObjects{}
	if o.DocumentStorage != nil {
		s3, err := mediapg.NewS3(*o.DocumentStorage)
		if err != nil {
			return err
		}
		objects = s3
	}
	mediaTokens, err := mediapg.NewTokens(o.Origin, o.CursorSecret)
	if err != nil {
		return err
	}
	originals, err := media.NewService(media.Config{Enabled: true, Authority: documentpg.MediaAuthority{Pool: o.Pool, Authority: authority}, Objects: objects,
		Inspector: mediapg.JPEGInspector{}, Tokens: mediaTokens, Audit: mediapg.AccessAudit{Runner: auditStore}, Now: time.Now, NewID: jobs.NewUUIDv4})
	if err != nil {
		return err
	}
	uploads, err := documentpg.NewUploadTokens(o.Origin, o.CursorSecret)
	if err != nil {
		return err
	}
	// EMP-DOC-003: the closed registry of linkable kinds, each resolved by
	// the module that owns the target record (EDR-162).
	targets := documentapp.Targets{documentdomain.Asset: fleetpg.DocumentTargets{}, documentdomain.MeterReading: fleetpg.DocumentTargets{},
		documentdomain.Employee: member.EmployeeDocumentTargets{}}
	store := &documentpg.Store{Pool: o.Pool, Authority: authority, Audit: auditStore, Targets: targets}
	// EMP-DOC-014: the ingress runs the same safety check as the queued job,
	// over the same bucket and inspector, so STORED never skips it.
	checker := documentapp.Checker{Repository: store, Objects: objects, Inspector: mediapg.JPEGInspector{}, NewID: jobs.NewUUIDv4}
	service := documentapp.Service{Enabled: true, Repository: store, Links: store, Media: originals, Safety: checker, Tokens: uploads, Now: time.Now, NewID: jobs.NewUUIDv4}
	return documentweb.Register(router, service, cookies, gateway, authorization)
}

type orgRootResolver struct{ roots org.Repository }

func (r orgRootResolver) ResolveRoot(ctx context.Context, scope kernel.OrgScope) error {
	_, err := r.roots.Get(ctx, scope)
	return err
}

// creationAdmission spends the identity v1 write budget on a keyed account
// subject before an organization is created. It commits on its own, so a
// refused or rolled-back creation still counts; no raw account ID is stored.
func creationAdmission(o Options) orgweb.CreationAdmission {
	limiter := identitypg.NewLimiter(o.Pool, identity.DefaultQuotaPolicy())
	key := append([]byte(nil), o.CursorSecret...)
	return func(ctx context.Context, accountID string) error {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte("orgcreate.v1:" + accountID))
		retry, err := limiter.Check(ctx, "orgcreate:v1:"+hex.EncodeToString(mac.Sum(nil)), identity.OperationWrite)
		if errors.Is(err, identity.ErrQuotaExceeded) && retry > 0 {
			return &orgapi.RateLimitError{RetryAfter: retry}
		}
		if err != nil {
			return orgapi.Unavailable
		}
		return nil
	}
}

// Invalid opaque locators spend the existing identity v1 registration allowance
// (10/hour) in distinct keyed account, browser-session and socket-IP subjects.
// Each consumption commits independently of a refused business transaction.
func unknownInvitationAdmission(o Options) memberweb.UnknownInvitationAdmission {
	limiter := identitypg.NewLimiter(o.Pool, identity.DefaultQuotaPolicy())
	key := append([]byte(nil), o.CursorSecret...)
	return func(ctx context.Context, a memberapp.EmployeeActor, address string) error {
		host, _, e := net.SplitHostPort(address)
		if e != nil {
			return orgapi.Unavailable
		}
		ip, e := identity.HashIPSubject("invitation.v1", key, host)
		if e != nil {
			return orgapi.Unavailable
		}
		subjects := []string{ip}
		for i, value := range []string{a.AccountID, a.SessionID} {
			mac := hmac.New(sha256.New, key)
			if i == 0 {
				mac.Write([]byte("invitation.unknown.account.v1:"))
			} else {
				mac.Write([]byte("invitation.unknown.browser.v1:"))
			}
			mac.Write([]byte(value))
			subjects = append(subjects, "invitation:unknown:v1:"+hex.EncodeToString(mac.Sum(nil)))
		}
		for _, subject := range subjects {
			retry, e := limiter.Check(ctx, subject, identity.OperationRegister)
			if errors.Is(e, identity.ErrQuotaExceeded) {
				return &orgapi.RateLimitError{RetryAfter: retry}
			}
			if e != nil {
				return orgapi.Unavailable
			}
		}
		return nil
	}
}
