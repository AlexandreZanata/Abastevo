package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
)

func TestFineListingActualHrefsAndVerifiedCoverage(t *testing.T) {
	body := []byte(`<a href="` + fineResource + `">2019</a>
<a href='` + FineResourcePrefix + `multas-aplicadas-2024.csv'>2021</a>
<a HREF="` + fineResource + `">duplicate</a>
<a href="` + FineResourcePrefix + `multas-aplicadas-2016a2019.csv">grouped</a>
<a href="` + FineResourcePrefix + `multas-2021-junho22.xlsx">spreadsheet</a>
<a href="https://evil.example/multas-aplicadas-2023.csv">foreign</a>
<a href="` + fineResource + `?secret=x">query</a>`)
	refs, e := ParseFineListingPage(FineListingPage, body)
	if e != nil || len(refs) != 4 || refs[0].Period != "2021" || refs[1].Period != "2016-2019" || refs[2].Period != "2024" || refs[3].Period != "2025" || refs[3].URL != fineResource {
		t.Fatal("listing invented coverage or trusted anchor labels", refs, e)
	}
	for _, page := range []string{FineListingPage + "?x", FineListingPage + "#x", strings.Replace(FineListingPage, "www.gov.br", "www.gov.br:443", 1), strings.Replace(FineListingPage, "https:", "http:", 1), FineListingPage + "/other"} {
		if _, e = ParseFineListingPage(page, body); e == nil {
			t.Fatal("foreign listing accepted")
		}
	}
	for _, raw := range [][]byte{nil, []byte(`<a href="multas-aplicadas-2017.csv">unverified</a>`), []byte(strings.Repeat("x", MaxListingBytes+1))} {
		if _, e = ParseFineListingPage(FineListingPage, raw); e == nil {
			t.Fatal("empty/unsupported/oversized listing accepted")
		}
	}
}

func TestFineInspectionPinsPrivateContentAndCancellation(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "source.csv")
	content := []byte("synthetic private content")
	if e := os.WriteFile(filename, content, 0600); e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(content)
	id := prepared.InputIdentity{SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(content))}
	ref := FineRef{URL: fineResource, Period: "2025"}
	entry, e := InspectFineFile(context.Background(), filename, ref, id)
	if e != nil || entry.SHA256 != id.SHA256 || entry.Label != "2025" || entry.Bytes != uint64(id.Bytes) {
		t.Fatal("identity not preserved", entry, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = InspectFineFile(ctx, filename, ref, id); e == nil {
		t.Fatal("cancelled inspection accepted")
	}
	if e = os.WriteFile(filename, []byte("altered private content"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = InspectFineFile(context.Background(), filename, ref, id); e == nil {
		t.Fatal("altered source accepted")
	}
	if e = os.WriteFile(filename, content, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(filename, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = InspectFineFile(context.Background(), filename, ref, id); e == nil {
		t.Fatal("public source accepted")
	}
	if e = os.Chmod(filename, 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "link.csv")
	if e = os.Symlink(filename, link); e != nil {
		t.Fatal(e)
	}
	if _, e = InspectFineFile(context.Background(), link, ref, id); e == nil {
		t.Fatal("symlink accepted")
	}
}
