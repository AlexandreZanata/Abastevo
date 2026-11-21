package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/application"
	capdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
)

// enterpriseArgs is one parsed capability command (B-BR-EMP-560/561,
// EDR-367/368). Parsing is pure so every refusal before I/O is unit-tested.
type enterpriseArgs struct {
	action   string // "set" or "show"
	operator string // canonical UUID recorded as the audit actor
	org      kernel.OrgScope
	change   capdomain.Change
}

// capabilityResult is the only output: no credential or DSN is printed.
type capabilityResult struct {
	org, capability   string
	enabled, replayed bool
	revision          int64
}

var (
	errOrganizationNotFound = errors.New("organization not found")
	errUnsafeLogin          = errors.New("refused: the database login is not the ordinary runtime role; run inside the api container after activation, never with the owner DSN")
	errCapabilityUsage      = errors.New("usage: ops enterprise capability show|set --org <uuid> --capability <code> [--enabled|--disabled --operation-id <uuid> --expected-revision <n>] [--operator <uuid>]")
)

func capabilityCatalog() (capdomain.Catalog, error) {
	return capdomain.NewCatalog(enterprisesuite.CapabilityCodes())
}

func canonicalUUID(raw string) (kernel.OrgID, bool) {
	id, err := kernel.ParseOrgID(raw)
	return id, err == nil && id.String() == raw
}

func parseEnterpriseArgs(args []string, getenv func(string) string) (enterpriseArgs, error) {
	if len(args) < 2 || args[0] != "capability" || (args[1] != "set" && args[1] != "show") {
		return enterpriseArgs{}, errCapabilityUsage
	}
	fs := flag.NewFlagSet("enterprise capability", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var org, code, operation, expected, operator string
	var enable, disable bool
	fs.StringVar(&org, "org", "", "organization UUID")
	fs.StringVar(&code, "capability", "", "registered capability code")
	fs.StringVar(&operator, "operator", "", "accountable operator UUID (or ANPFUEL_OPERATOR_ID)")
	if args[1] == "set" {
		fs.BoolVar(&enable, "enabled", false, "turn the capability on")
		fs.BoolVar(&disable, "disabled", false, "turn the capability off")
		fs.StringVar(&operation, "operation-id", "", "operation UUID, reused only to retry the same command")
		fs.StringVar(&expected, "expected-revision", "", "current revision (0 for the first state)")
	}
	if err := fs.Parse(args[2:]); err != nil || fs.NArg() != 0 {
		return enterpriseArgs{}, errCapabilityUsage
	}
	who, err := resolveOperatorID(operator, getenv)
	if err != nil {
		return enterpriseArgs{}, err
	}
	if _, ok := canonicalUUID(who); !ok {
		return enterpriseArgs{}, errors.New("the operator identity of a capability change must be a canonical UUID")
	}
	id, ok := canonicalUUID(strings.TrimSpace(org))
	if !ok || id.String() != org {
		return enterpriseArgs{}, errors.New("enterprise capability requires a canonical --org UUID")
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return enterpriseArgs{}, errors.New("enterprise capability requires a canonical --org UUID")
	}
	catalog, err := capabilityCatalog()
	if err != nil {
		return enterpriseArgs{}, err
	}
	if catalog.Validate(code) != nil {
		return enterpriseArgs{}, fmt.Errorf("unknown capability %q", code)
	}
	parsed := enterpriseArgs{action: args[1], operator: who, org: scope, change: capdomain.Change{Capability: code}}
	if parsed.action == "show" {
		return parsed, nil
	}
	if enable == disable {
		return enterpriseArgs{}, errors.New("enterprise capability set requires exactly one of --enabled or --disabled")
	}
	revision, err := strconv.ParseInt(expected, 10, 64)
	if err != nil || expected != strconv.FormatInt(revision, 10) {
		return enterpriseArgs{}, errors.New("enterprise capability set requires --expected-revision <n>")
	}
	parsed.change = capdomain.Change{OperationID: operation, Capability: code, Enabled: enable, ExpectedRevision: revision}
	if parsed.change.Validate(catalog) != nil {
		return enterpriseArgs{}, errors.New("enterprise capability set requires a canonical --operation-id UUID and a non-negative --expected-revision")
	}
	return parsed, nil
}

// operatorAuthority is the capability service's authority port for the
// restricted operator tool: it admits only Manage and Read, for an
// organization that exists, and never a product intent (EDR-368).
type operatorAuthority struct{ roots *orgpg.Store }

func (a operatorAuthority) Authorize(ctx context.Context, scope kernel.OrgScope, _ capdomain.Actor, _ string, intent capdomain.Intent) error {
	if intent != capdomain.Manage && intent != capdomain.Read {
		return capdomain.ErrDenied
	}
	_, err := a.roots.Get(ctx, scope)
	switch {
	case err == nil, errors.Is(err, orgapp.ErrUninitialized):
		return nil
	case errors.Is(err, orgapp.ErrNotFound):
		return errOrganizationNotFound
	}
	return capdomain.ErrUnavailable
}

// executeCapability composes the existing audited capability service on the
// given pool, after proving that the login passes the organization role guard.
func executeCapability(ctx context.Context, pool *pgxpool.Pool, in enterpriseArgs) (capabilityResult, error) {
	if err := database.WithOrgTx(ctx, pool, in.org, func(pgx.Tx) error { return nil }); err != nil {
		if errors.Is(err, database.ErrUnsafeOrgRole) {
			return capabilityResult{}, errUnsafeLogin
		}
		return capabilityResult{}, errors.New("enterprise database unavailable")
	}
	catalog, err := capabilityCatalog()
	if err != nil {
		return capabilityResult{}, err
	}
	audits, err := audit.NewCatalog(cappg.AuditDefinitions())
	if err != nil {
		return capabilityResult{}, err
	}
	service := capapp.Service{
		Enabled:    true,
		Catalog:    catalog,
		Authorizer: operatorAuthority{roots: orgpg.NewStore(pool)},
		Repository: cappg.NewStore(pool, catalog, auditpg.NewStore(pool, audits)),
	}
	actor := capdomain.Actor{AccountID: in.operator}
	replayed := false
	if in.action == "set" {
		if replayed, err = service.Set(ctx, in.org, actor, in.change); err != nil {
			return capabilityResult{}, err
		}
	}
	state, err := service.State(ctx, in.org, actor, in.change.Capability)
	if err != nil {
		return capabilityResult{}, err
	}
	return capabilityResult{org: in.org.OrgID().String(), capability: in.change.Capability, enabled: state.Enabled, replayed: replayed, revision: state.Revision}, nil
}

func describeCapabilityError(err error) error {
	switch {
	case errors.Is(err, errUnsafeLogin), errors.Is(err, errOrganizationNotFound):
		return err
	case errors.Is(err, capdomain.ErrConflict):
		return fmt.Errorf("conflict: the expected revision is stale; read it with show: %w", err)
	case errors.Is(err, capdomain.ErrReused):
		return fmt.Errorf("operation ID already used for another command: %w", err)
	case errors.Is(err, capdomain.ErrInvalid):
		return fmt.Errorf("invalid capability command: %w", err)
	}
	return fmt.Errorf("capability change unavailable: %w", err)
}

// runEnterprise opens the database named by this process's configuration
// (the runtime DSN inside the api container) and prints one bounded line.
func runEnterprise(args []string, getenv func(string) string, out io.Writer) error {
	if len(args) > 0 && args[0] == "data" {
		return runEnterpriseData(args[1:], out)
	}
	if len(args) > 0 && args[0] == "trial" {
		return runEnterpriseTrial(args[1:], getenv, out)
	}
	parsed, err := parseEnterpriseArgs(args, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return errors.New("enterprise database unavailable")
	}
	defer pool.Close()
	result, err := executeCapability(ctx, pool.Underlying(), parsed)
	if err != nil {
		return describeCapabilityError(err)
	}
	_, err = fmt.Fprintf(out, "organization %s capability %s enabled=%t revision=%d replayed=%t\n", result.org, result.capability, result.enabled, result.revision, result.replayed)
	return err
}
