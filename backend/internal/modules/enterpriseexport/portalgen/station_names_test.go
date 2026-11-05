package portalgen

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

func TestPortalCanonicalNamesStaySearchableAndRenderSecondaryLegalName(t *testing.T) {
	snapshot := writeFixtureWith(t, func(stations *[]enterpriseexport.Station, _ *[]enterpriseexport.Price) {
		(*stations)[0].LegalName = "Comercial Sintética LTDA 12345678909"
		(*stations)[1].LegalName = "  posto   BETA "
	})
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	html := readPortal(t, out, "postos/"+stA+"/index.html")
	if !strings.Contains(html, "<h1>Posto Alfa</h1>") || !strings.Contains(html, `class="station-legal-name muted">Comercial Sintética LTDA<`) || strings.Contains(html, "12345678909") {
		t.Fatal("primary/secondary identity or privacy failed")
	}
	if strings.Contains(readPortal(t, out, "postos/"+stB+"/index.html"), `class="station-legal-name muted"`) {
		t.Fatal("same legal name rendered twice")
	}
	var prefix searchPrefixShard
	readIndexJSON(t, out, "p/n-sin.json", &prefix)
	if prefix.Count != 1 || prefix.Stations[0].Name != "Posto Alfa" || prefix.Stations[0].LegalName != "Comercial Sintética LTDA" {
		t.Fatal("legal-name prefix cannot find trade name", prefix)
	}
	var prices searchPriceShard
	readIndexJSON(t, out, "precos/GASOLINE_REGULAR.json", &prices)
	found := false
	for _, row := range prices.Rows {
		if row.ID == stA {
			found = row.LegalName == "Comercial Sintética LTDA"
		}
	}
	if !found {
		t.Fatal("price search lost secondary name")
	}
	if _, err := Verify(out, VerifyOptions{}); err != nil {
		t.Fatal(err)
	}
}
