package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/apicontract"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

// This committed inventory is an independent test oracle, never runtime policy.
// A newly routed permission must be classified and gain real-request matrix cases.
type authorizationInventory struct {
	SchemaVersion           int                            `json:"schema_version"`
	RolesVersion            string                         `json:"roles_version"`
	PermissionsVersion      string                         `json:"permissions_version"`
	Permissions             []authorizationPermission      `json:"permissions"`
	SpecialRoutes           []authorizationSpecialRoute    `json:"special_routes"`
	UnavailableStationRoles []authorizationUnavailableRole `json:"unavailable_station_roles"`
}
type authorizationPermission struct {
	Permission      string   `json:"permission"`
	Status          string   `json:"status"`
	Representative  string   `json:"representative_route"`
	Routes          []string `json:"routes"`
	Reason          string   `json:"reason"`
	OperationType   string   `json:"operation_type"`
	OperationSchema int64    `json:"operation_schema"`
}
type authorizationSpecialRoute struct {
	Route    string `json:"route"`
	Boundary string `json:"boundary"`
}
type authorizationUnavailableRole struct {
	Role   string `json:"role"`
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
}

func loadAuthorizationInventory(t *testing.T) authorizationInventory {
	t.Helper()
	path := filepath.Join("..", "..", "..", "contracts", "testdata", "enterprise", "authorization-routes-v1.json")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var inventory authorizationInventory
	if err := decoder.Decode(&inventory); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatal("trailing inventory input", err)
	}
	return inventory
}

func authorizationInventoryMaps(inventory authorizationInventory) (map[string]string, map[string]string, error) {
	catalog := access.SystemCatalog()
	rolesVersion, permissionsVersion := catalog.Versions()
	if inventory.SchemaVersion != 1 || inventory.RolesVersion != rolesVersion || inventory.PermissionsVersion != permissionsVersion || len(inventory.Permissions) != 85 || len(catalog.Permissions()) != 85 {
		return nil, nil, fmt.Errorf("authorization inventory version or permission count drift")
	}
	normal, special := map[string]string{}, map[string]string{}
	routed, signed, notRouted := 0, 0, 0
	var signedRoutes []string
	var keys []access.Permission
	for _, row := range inventory.Permissions {
		keys = append(keys, access.Permission(row.Permission))
		if row.Status != "SIGNED_OPERATION" && (row.OperationType != "" || row.OperationSchema != 0) {
			return nil, nil, fmt.Errorf("non-operation inventory includes signed operation fields")
		}
		switch row.Status {
		case "ROUTED":
			routed++
			if row.Reason != "" || row.Representative == "" || !slices.Contains(row.Routes, row.Representative) || !slices.IsSorted(row.Routes) {
				return nil, nil, fmt.Errorf("routed authorization inventory lacks a representative")
			}
			for _, route := range row.Routes {
				if !inventoryOrganizationRoute(route) || normal[route] != "" {
					return nil, nil, fmt.Errorf("invalid or duplicate authorization inventory route")
				}
				normal[route] = row.Permission
			}
		case "SIGNED_OPERATION":
			signed++
			if row.Reason != "" || row.OperationType != "meter.reading.record" || row.OperationSchema != 1 || row.Permission != "meter.reading.record" || len(row.Routes) != 1 || row.Representative != "POST /v1/orgs/{org_id}/sync/operations" || row.Routes[0] != row.Representative {
				return nil, nil, fmt.Errorf("signed-operation inventory lacks its exact source-owned descriptor")
			}
			signedRoutes = append(signedRoutes, row.Representative)
		case "NOT_YET_ROUTED":
			notRouted++
			if len(row.Routes) != 0 || row.Representative != "" || strings.TrimSpace(row.Reason) == "" {
				return nil, nil, fmt.Errorf("unrouted authorization obligation missing")
			}
		default:
			return nil, nil, fmt.Errorf("unknown authorization inventory status")
		}
	}
	if !slices.Equal(keys, catalog.Permissions()) || routed != 15 || signed != 1 || notRouted != 69 || len(normal) != 82 {
		return nil, nil, fmt.Errorf("authorization inventory does not cover every catalog permission")
	}
	expectedBoundaries := map[string]int{"ACCOUNT_CONTEXT": 2, "OWNED_SETUPS": 1, "COMPANY_SESSION": 3, "ORGANIZATION_CREATION": 1, "PERSONAL_INVITATION": 3, "SELF_MEMBERSHIP": 2, "SIGNED_DEVICE": 2, "SELF_DEVICE": 2, "DEVICE_CONTACT": 1, "SIGNED_BATCH": 1, "SIGNED_REVIEW_RESULTS": 1}
	actualBoundaries := map[string]int{}
	var specialOrder []string
	for _, row := range inventory.SpecialRoutes {
		if !inventoryOrganizationRoute(row.Route) || normal[row.Route] != "" || special[row.Route] != "" || expectedBoundaries[row.Boundary] == 0 {
			return nil, nil, fmt.Errorf("invalid special authorization boundary")
		}
		special[row.Route] = row.Boundary
		specialOrder = append(specialOrder, row.Route)
		actualBoundaries[row.Boundary]++
	}
	if len(special) != 19 || !slices.IsSorted(specialOrder) || len(actualBoundaries) != len(expectedBoundaries) {
		return nil, nil, fmt.Errorf("special authorization inventory coverage drift")
	}
	for boundary, count := range expectedBoundaries {
		if actualBoundaries[boundary] != count {
			return nil, nil, fmt.Errorf("special authorization boundary count drift")
		}
	}
	for _, route := range signedRoutes {
		if special[route] != "SIGNED_BATCH" {
			return nil, nil, fmt.Errorf("signed operation is not behind the actual signed batch boundary")
		}
	}
	var unavailable []string
	for _, row := range inventory.UnavailableStationRoles {
		if row.Scope != "STATION" || strings.TrimSpace(row.Reason) == "" {
			return nil, nil, fmt.Errorf("station scope obligation missing")
		}
		unavailable = append(unavailable, row.Role)
		if _, err := catalog.Lookup(access.Role(row.Role), "org.ownership.respond"); err != nil {
			return nil, nil, fmt.Errorf("unknown unavailable station role")
		}
	}
	if !slices.Equal(unavailable, []string{"STATION_ADMIN", "STATION_ATTENDANT", "STATION_MANAGER"}) {
		return nil, nil, fmt.Errorf("unavailable station role inventory drift")
	}
	return normal, special, nil
}

func TestAuthorizationInventoryMatchesRegisteredMeterDescriptor(t *testing.T) {
	inventory := loadAuthorizationInventory(t)
	if _, _, err := authorizationInventoryMaps(inventory); err != nil {
		t.Fatal(err)
	}
	// This inspects only actual source-owned descriptor registration. The
	// separate signed HTTP matrix proves ordinary current runtime authority.
	handler, err := fleetpg.NewMeterIntakeHandler(&orgpg.MeterAuthority{}, &fleetpg.MeterStore{})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := syncapp.NewRegistry(handler)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range inventory.Permissions {
		if row.Status == "SIGNED_OPERATION" {
			registered, ok := registry.Lookup(row.OperationType)
			if !ok {
				t.Fatal("inventoried signed producer is not registered")
			}
			descriptor := registered.Descriptor()
			if descriptor.Validate() != nil || descriptor.Type != row.OperationType || descriptor.Schema != row.OperationSchema || descriptor.WritePolicy.Permission != access.Permission(row.Permission) {
				t.Fatal("signed permission/type/schema drift from actual descriptor")
			}
		}
	}
}

func inventoryOrganizationRoute(route string) bool {
	return strings.HasPrefix(route, "POST ") && orgapi.IsOrganizationPath(strings.TrimPrefix(route, "POST ")) && !strings.ContainsAny(route, "\r\n\t")
}

func authorizationSpecialBoundary(handler http.Handler) string {
	// The canonical gate verifies each descriptor's exact proof flags and action.
	switch handler.(type) {
	case orgapi.OwnedSetupsDescriptor:
		return "OWNED_SETUPS"
	case orgapi.AccountContextDescriptor:
		return "ACCOUNT_CONTEXT"
	case orgapi.CompanySessionDescriptor:
		return "COMPANY_SESSION"
	case orgapi.OrganizationCreationDescriptor:
		return "ORGANIZATION_CREATION"
	case orgapi.PersonalInvitationDescriptor:
		return "PERSONAL_INVITATION"
	case orgapi.SelfMembershipDescriptor:
		return "SELF_MEMBERSHIP"
	case orgapi.SignedDeviceDescriptor:
		return "SIGNED_DEVICE"
	case orgapi.DeviceContactDescriptor:
		return "DEVICE_CONTACT"
	case orgapi.SignedBatchDescriptor:
		return "SIGNED_BATCH"
	case orgapi.SignedReviewResultsDescriptor:
		return "SIGNED_REVIEW_RESULTS"
	case orgapi.SelfDeviceDescriptor:
		return "SELF_DEVICE"
	default:
		return ""
	}
}

func validateAuthorizationInventory(router chi.Routes, document *openapi3.T, inventory authorizationInventory) error {
	normal, special, err := authorizationInventoryMaps(inventory)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	err = chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !orgapi.IsOrganizationPath(path) {
			return nil
		}
		route := method + " " + path
		if seen[route] {
			return fmt.Errorf("duplicate organization route")
		}
		seen[route] = true
		if permission := normal[route]; permission != "" {
			descriptor, ok := handler.(orgapi.Descriptor)
			if !ok || descriptor.OrganizationRule().Permission != permission {
				return fmt.Errorf("route permission mismatch: %s", route)
			}
			if completion, ok := handler.(orgapi.AuthorityCompletion); !ok || !completion.CompletesAuthority() {
				return fmt.Errorf("missing authority completion: %s", route)
			}
		} else if boundary := special[route]; boundary != "" {
			if authorizationSpecialBoundary(handler) != boundary {
				return fmt.Errorf("special boundary mismatch: %s", route)
			}
		} else {
			return fmt.Errorf("unclassified organization route: %s", route)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, routes := range []map[string]string{normal, special} {
		for route := range routes {
			if !seen[route] {
				return fmt.Errorf("inventoried route missing: %s", route)
			}
		}
	}
	return apicontract.ValidateOrganizationRoutes(router, document)
}

func TestAuthorizationInventoryCoversFullProductionRouter(t *testing.T) {
	inventory := loadAuthorizationInventory(t)
	for key, value := range map[string]string{"ANPFUEL_ENV": "development", "ANPFUEL_DATABASE_URL": "postgres://synthetic@127.0.0.1:1/synthetic?sslmode=disable", "ANPFUEL_CANONICAL_HOST": "company.example.invalid", "ANPFUEL_ENTERPRISE_ENABLED": "true", "ANPFUEL_FORUM_ENABLED": "false", "ANPFUEL_MAIL_MODE": "off", "ANPFUEL_METRICS_ADDR": "", "ANPFUEL_MAP_DIST_DIR": "", "ANPFUEL_MAP_DIST_PUBLIC_KEYS": "", "ANPFUEL_ADMIN_KEY_SHA256": "", "ANPFUEL_ADMIN_KEY_SHA256_NEXT": "", "ANPFUEL_R2_ENDPOINT": "", "ANPFUEL_R2_BUCKET": "", "ANPFUEL_R2_ACCESS_KEY_ID": "", "ANPFUEL_R2_SECRET_ACCESS_KEY": ""} {
		t.Setenv(key, value)
	}
	stopped := errors.New("authorization inventory composition checkpoint")
	observed := false
	err := runWithRouterCheck(func(router chi.Router) error {
		observed = true
		document, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "contracts", "openapi", "v1.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		if err := chi.Walk(router, func(string, string, http.Handler, ...func(http.Handler) http.Handler) error { total++; return nil }); err != nil {
			t.Fatal(err)
		}
		if total < 100 {
			t.Fatal("not the full production router", total)
		}
		if err := validateAuthorizationInventory(router, document, inventory); err != nil {
			t.Fatal(err)
		}
		t.Run("reject route and policy mutations", func(t *testing.T) { verifyAuthorizationRouteMutations(t, router, document, inventory) })
		return stopped
	})
	if !observed || !errors.Is(err, stopped) {
		t.Fatal("full router checkpoint not reached", err)
	}
}

type authorizationRoute struct {
	method, path string
	handler      http.Handler
}
type authorizationDescriptorMutation struct {
	http.Handler
	rule       orgapi.Rule
	completion bool
}

func (m authorizationDescriptorMutation) OrganizationRule() orgapi.Rule { return m.rule }
func (m authorizationDescriptorMutation) CompletesAuthority() bool      { return m.completion }

func verifyAuthorizationRouteMutations(t *testing.T, actual chi.Routes, document *openapi3.T, inventory authorizationInventory) {
	t.Helper()
	var routes []authorizationRoute
	if err := chi.Walk(actual, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		if orgapi.IsOrganizationPath(path) {
			routes = append(routes, authorizationRoute{method, path, handler})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	readPath := "/v1/orgs/{org_id}/settings/read"
	for _, test := range []struct {
		name, expected string
		mutate         func(*chi.Mux, authorizationRoute) bool
		extra          bool
	}{
		{"unguarded extra route", "unclassified organization route", nil, true},
		{"missing actual route", "inventoried route missing", func(_ *chi.Mux, route authorizationRoute) bool { return route.path == readPath }, false},
		{"permission changed", "route permission mismatch", func(router *chi.Mux, route authorizationRoute) bool {
			if route.path != readPath {
				return false
			}
			rule := route.handler.(orgapi.Descriptor).OrganizationRule()
			rule.Permission = "auth.role.assign"
			router.Method(route.method, route.path, authorizationDescriptorMutation{route.handler, rule, true})
			return true
		}, false},
		{"newly routed permission", "route permission mismatch", func(router *chi.Mux, route authorizationRoute) bool {
			if route.path != readPath {
				return false
			}
			rule := route.handler.(orgapi.Descriptor).OrganizationRule()
			rule.Permission = "trip.trip.start"
			router.Method(route.method, route.path, authorizationDescriptorMutation{route.handler, rule, true})
			return true
		}, false},
		{"missing completion", "missing authority completion", func(router *chi.Mux, route authorizationRoute) bool {
			if route.path != readPath {
				return false
			}
			rule := route.handler.(orgapi.Descriptor).OrganizationRule()
			router.Method(route.method, route.path, authorizationDescriptorMutation{route.handler, rule, false})
			return true
		}, false},
		{"special boundary replaced", "special boundary mismatch", func(router *chi.Mux, route authorizationRoute) bool {
			if route.path != "/v1/org-access/contexts/read" {
				return false
			}
			router.Method(route.method, route.path, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			return true
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := chi.NewRouter()
			for _, route := range routes {
				if test.mutate != nil && test.mutate(router, route) {
					continue
				}
				router.Method(route.method, route.path, route.handler)
			}
			if test.extra {
				router.Post("/v1/orgs/{org_id}/authorization-unclassified/read", func(http.ResponseWriter, *http.Request) {})
			}
			err := validateAuthorizationInventory(router, document, inventory)
			if err == nil || !strings.Contains(err.Error(), test.expected) {
				t.Fatal("route mutation escaped its intended guard", err)
			}
		})
	}
	t.Run("extra guarded route for existing permission", func(t *testing.T) {
		router := chi.NewRouter()
		for _, route := range routes {
			router.Method(route.method, route.path, route.handler)
			if route.path == readPath {
				router.Method(http.MethodPost, "/v1/orgs/{org_id}/authorization-unclassified/read", route.handler)
			}
		}
		err := validateAuthorizationInventory(router, document, inventory)
		if err == nil || !strings.Contains(err.Error(), "unclassified organization route") {
			t.Fatal("extra guarded route was silently sampled", err)
		}
	})
}

func TestAuthorizationInventoryRejectsMissingOrFalseObligations(t *testing.T) {
	source := loadAuthorizationInventory(t)
	for _, test := range []struct {
		name   string
		change func(*authorizationInventory)
	}{
		{"schema", func(i *authorizationInventory) { i.SchemaVersion = 2 }},
		{"catalog version", func(i *authorizationInventory) { i.PermissionsVersion = "org-permissions-v1" }},
		{"missing permission", func(i *authorizationInventory) { i.Permissions = i.Permissions[1:] }},
		{"duplicate permission", func(i *authorizationInventory) { i.Permissions[1].Permission = i.Permissions[0].Permission }},
		{"unknown status", func(i *authorizationInventory) { i.Permissions[0].Status = "PASSED" }},
		{"operation fields on future permission", func(i *authorizationInventory) { i.Permissions[0].OperationType = "meter.reading.record" }},
		{"wrong signed schema", func(i *authorizationInventory) {
			for j := range i.Permissions {
				if i.Permissions[j].Status == "SIGNED_OPERATION" {
					i.Permissions[j].OperationSchema = 2
					return
				}
			}
		}},
		{"wrong signed type", func(i *authorizationInventory) {
			for j := range i.Permissions {
				if i.Permissions[j].Status == "SIGNED_OPERATION" {
					i.Permissions[j].OperationType = "invented.record"
					return
				}
			}
		}},
		{"signed permission as cookie route", func(i *authorizationInventory) {
			for j := range i.Permissions {
				if i.Permissions[j].Status == "SIGNED_OPERATION" {
					i.Permissions[j].Representative = "POST /v1/orgs/{org_id}/fleet/assets/list"
					return
				}
			}
		}},
		{"missing future reason", func(i *authorizationInventory) { i.Permissions[0].Reason = "" }},
		{"invented representative", func(i *authorizationInventory) {
			for j := range i.Permissions {
				if i.Permissions[j].Status == "ROUTED" {
					i.Permissions[j].Representative = "POST /v1/orgs/{org_id}/invented"
					return
				}
			}
		}},
		{"unknown special boundary", func(i *authorizationInventory) { i.SpecialRoutes[0].Boundary = "BYPASS" }},
		{"missing special route", func(i *authorizationInventory) { i.SpecialRoutes = i.SpecialRoutes[1:] }},
		{"missing station obligation", func(i *authorizationInventory) { i.UnavailableStationRoles = i.UnavailableStationRoles[1:] }},
		{"false station scope", func(i *authorizationInventory) { i.UnavailableStationRoles[0].Scope = "ORG" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			var changed authorizationInventory
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			test.change(&changed)
			if _, _, err := authorizationInventoryMaps(changed); err == nil {
				t.Fatal("incomplete authorization inventory accepted")
			}
		})
	}
}
