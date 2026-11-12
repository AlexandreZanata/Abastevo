// Package enterprisesuite composes the private enterprise entries. Runtime
// credentials/grants, operator flags and legal approval remain independent.
package enterprisesuite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	companyweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
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
}

// Register performs no I/O or provisioning. The default mounts no private
// entry; enabled commands still require an ordinary role and current proof.
func Register(router chi.Router, o Options) error {
	if !o.Enabled {
		return nil
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
		{Class: "idn.employee", Permissions: []string{"idn.employee.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic employee register only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."},
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
