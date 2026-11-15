package apicontract

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters/lists"
	exportapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	exportdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

// EMP-RPT-002 (B-BR-EMP-255/257): the contract names the admitted list kinds,
// their closed filter objects, the package delivered and the kinds that stay
// unavailable; each documented filter schema matches the server's own
// canonicalization.
func TestListExportKindsAreDocumentedExactly(t *testing.T) {
	doc := loadSpec(t)
	kind := doc.Components.Schemas["EnterpriseExportKind"]
	if kind == nil || kind.Value == nil {
		t.Fatal("export kind schema missing")
	}
	for _, phrase := range []string{lists.AssetListKind, lists.ReadingListKind, lists.EmployeeListKind, ".xlsx", "EXPORT_FORMAT_UNAVAILABLE"} {
		if !strings.Contains(kind.Value.Description, phrase) {
			t.Fatal("export kind description misses", phrase)
		}
	}
	if len(kind.Value.Enum) != 0 {
		t.Fatal("an enum would refuse unadmitted kinds before the server answers EXPORT_FORMAT_UNAVAILABLE")
	}
	request := doc.Paths.Value("/v1/orgs/{org_id}/reports/exports/request").Post
	content := doc.Paths.Value("/v1/orgs/{org_id}/reports/exports/{export_id}/content").Post
	if strings.Contains(request.Description, "admits no real export format") || strings.Contains(content.Description, "admits no real export format") {
		t.Fatal("contract still says no real format is admitted")
	}
	for _, phrase := range []string{exportdomain.ListMediaType, exportdomain.ListManifestName, "inert", "VALIDATION_FAILED"} {
		if !strings.Contains(request.Description+content.Description, phrase) {
			t.Fatal("list package undocumented", phrase)
		}
	}
	formats := map[string]exportapp.Format{}
	for _, f := range lists.New(nil, nil) {
		formats[f.Kind()] = f
	}
	for name, tc := range map[string]struct {
		kind string
		keys []string
	}{
		"EnterpriseAssetListExportParameters":        {lists.AssetListKind, []string{"kind", "state"}},
		"EnterpriseMeterReadingListExportParameters": {lists.ReadingListKind, []string{"asset_id", "meter_kind", "occurred_from", "occurred_to", "state"}},
		"EnterpriseEmployeeListExportParameters":     {lists.EmployeeListKind, []string{"state"}},
	} {
		t.Run(name, func(t *testing.T) {
			s := doc.Components.Schemas[name]
			if s == nil || s.Value == nil || s.Value.Example == nil || s.Value.AdditionalProperties.Has == nil || *s.Value.AdditionalProperties.Has || len(s.Value.Required) != 0 {
				t.Fatal("closed optional filter schema missing")
			}
			var keys []string
			for key := range s.Value.Properties {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			if !slices.Equal(keys, tc.keys) || !strings.Contains(s.Value.Description, tc.kind) {
				t.Fatal("filter keys", keys)
			}
			raw, err := json.Marshal(s.Value.Example)
			if err != nil || s.Value.VisitJSON(s.Value.Example) != nil {
				t.Fatal("invalid filter example", err)
			}
			canonical, err := exportdomain.CanonicalParameters(raw)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := formats[tc.kind].Canonical(canonical); err != nil {
				t.Fatal("documented example refused by the server", string(raw), err)
			}
			var extra map[string]any
			_ = json.Unmarshal(raw, &extra)
			extra["include_contact"] = true
			if s.Value.VisitJSON(extra) == nil {
				t.Fatal("unknown filter accepted by the contract")
			}
			again, _ := json.Marshal(extra)
			canonical, _ = exportdomain.CanonicalParameters(again)
			if _, err := formats[tc.kind].Canonical(canonical); err == nil {
				t.Fatal("unknown filter accepted by the server")
			}
		})
	}
}
