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
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	proofauth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgquota"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Enabled      bool
	Origin       string
	CursorSecret []byte
	Pool         *pgxpool.Pool
	Accounts     *account.Service
	DeviceLimit  int64
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
		{Class: "idn.invitation", Permissions: []string{"idn.invitation.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic invitation writes use the existing identity v1 write allowance on every axis; production measurement remains required."},
		{Class: "idn.membership", Permissions: []string{"idn.membership.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic membership lifecycle uses identity v1 write200/hour on every axis; production measurement remains required."},
		{Class: "idn.employee", Permissions: []string{"idn.employee.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic employee register only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
		{Class: "idn.device", Permissions: []string{"idn.credential.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic scoped device enrollment and revocation inherit identity v1 write200/hour on every axis; current source membership precedes admission and production measurement remains required."},
		{Class: "auth.role", Permissions: []string{"auth.role.assign"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic role management only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
		{Class: "org.ownership_transfer", Permissions: []string{"org.ownership.transfer", "org.ownership.respond"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic ownership transfer only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
	})
	if err != nil {
		return err
	}
	gateway := orgapi.Gateway{Enabled: true, Origin: o.Origin, CursorSecret: o.CursorSecret, Now: time.Now, WriteAdmission: admission}
	authorization := access.Authorization{Source: roles, Capabilities: accesspg.PilotEntitlements{Enabled: true, Catalog: capabilities, States: flags}, Records: orgweb.SetupRecords{}, Now: time.Now}
	if err := orgweb.Register(router, org.Onboarding{Enabled: true, Repository: store, NewID: jobs.NewUUIDv4}, cookies, gateway, authorization, roots, creationAdmission(o)); err != nil {
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

	cookies.RegisterRoutes(router)
	return nil
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
