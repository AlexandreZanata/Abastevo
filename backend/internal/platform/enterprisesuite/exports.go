package enterprisesuite

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/go-chi/chi/v5"

	companyweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	intelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/adapters"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	datapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	exportweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters/lists"
	exportapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

var errExportComposition = errors.New("enterprisesuite: incomplete export composition")

// exportParts composes the installed formats and private object storage.
// Format readers recheck the requesting membership through the export authority.
func exportParts(o Options, requesters lists.Requesters) (exportapp.Registry, exportapp.Objects, error) {
	all := append(lists.New(o.Pool, requesters), intelpg.ExportFormats(o.Pool, requesters)...)
	if o.BillingEnabled {
		codes, err := capability.NewCatalog(CapabilityCodes())
		if err != nil {
			return exportapp.Registry{}, nil, err
		}
		all = append(all, datapg.MarketComparisonExport{Pool: o.Pool, Requesters: requesters,
			Plans: billpg.EntitlementStore{Pool: o.Pool, Capabilities: codes, Now: time.Now},
			Flags: cappg.NewStore(o.Pool, codes, nil)})
	}

	formats, err := exportapp.NewRegistry(append(all, o.ExportFormats...)...)
	if err != nil {
		return exportapp.Registry{}, nil, err
	}
	if o.ExportStorage == nil {
		return formats, exportpg.Unconfigured{}, nil
	}
	objects, err := exportpg.NewS3(*o.ExportStorage)
	if err != nil {
		return exportapp.Registry{}, nil, err
	}
	return formats, objects, nil
}

// exportCapabilityKey separates the capability key from the cursor key.
func exportCapabilityKey(secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("orgexport.capability.v1"))
	return mac.Sum(nil)
}

// registerExports mounts the export routes (EMP-RPT-001) on the shared
// company entry, authority and audit runner.
func registerExports(router chi.Router, o Options, cookies companyweb.Handler, gateway orgapi.Gateway, authority *orgpg.ExportAuthority, capabilities access.CapabilityGate, roles access.AuthoritySource, audit *auditpg.Store) error {
	formats, objects, err := exportParts(o, authority)
	if err != nil {
		return err
	}
	tokens, err := exportpg.NewTokens(o.Origin, exportCapabilityKey(o.CursorSecret))
	if err != nil {
		return err
	}
	service := exportapp.Service{Enabled: true, Repository: &exportpg.Store{Pool: o.Pool, Authority: authority, Audit: audit}, Formats: formats, Objects: objects,
		Tokens: tokens, Recorder: auditpg.AccessLog{Runner: audit}, NewID: jobs.NewUUIDv4, Now: time.Now}
	authorization := access.Authorization{Source: roles, Capabilities: capabilities, Records: exportweb.ExportRecords{}, Now: time.Now}
	return exportweb.RegisterExports(router, service, cookies, gateway, authorization)
}

// ExportHandlers composes the export build and cleanup queue handlers with
// the same authority, storage and formats as the routes (EMP-RPT-001). The
// disabled suite composes none.
func ExportHandlers(o Options) ([]jobs.Handler, error) {
	if !o.Enabled {
		return nil, nil
	}
	if o.Pool == nil {
		return nil, errExportComposition
	}
	// The worker re-evaluates recorded requesters only; it never runs the
	// session-bound company pipeline.
	authority := &orgpg.ExportAuthority{Memberships: member.NewStore(o.Pool), Roles: accesspg.NewStore(o.Pool)}
	formats, objects, err := exportParts(o, authority)
	if err != nil {
		return nil, err
	}
	worker := exportapp.Worker{Store: &exportpg.Store{Pool: o.Pool, Authority: authority}, Formats: formats, Objects: objects, NewID: jobs.NewUUIDv4, LeaseTTL: exportapp.DefaultLeaseTTL}
	return []jobs.Handler{exportpg.BuildHandler{Worker: worker}, exportpg.CleanupHandler{Worker: worker}}, nil
}
