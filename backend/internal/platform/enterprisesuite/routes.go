// Package enterprisesuite composes the private enterprise entries. Runtime
// credentials/grants, operator flags and legal approval remain independent.
package enterprisesuite

import (
	"time"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	companyweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
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
	store := &orgpg.OnboardingStore{Pool: o.Pool, Roots: roots, Memberships: member.NewStore(o.Pool), Roles: roles, Audit: auditStore, Capabilities: flags}
	allowance, err := identity.DefaultQuotaPolicy().Lookup(identity.OperationWrite)
	if err != nil {
		return err
	}
	admission, err := orgquota.New(o.Pool, o.CursorSecret, []orgquota.Definition{{Class: "org.setup", Permissions: []string{"org.settings.manage"}, Organization: allowance, Account: allowance, Device: allowance, Rationale: "Synthetic setup only: inherit identity v1 write 200/hour on every axis; production measurement and cleanup activation remain mandatory."}})
	if err != nil {
		return err
	}
	gateway := orgapi.Gateway{Enabled: true, Origin: o.Origin, CursorSecret: o.CursorSecret, Now: time.Now, WriteAdmission: admission}
	authorization := access.Authorization{Source: roles, Capabilities: accesspg.PilotEntitlements{Enabled: true, Catalog: capabilities, States: flags}, Records: orgweb.SetupRecords{}, Now: time.Now}
	if err := orgweb.Register(router, org.Onboarding{Enabled: true, Repository: store, NewID: jobs.NewUUIDv4}, cookies, gateway, authorization, roots); err != nil {
		return err
	}
	cookies.RegisterRoutes(router)
	return nil
}
