//go:build integration

package native

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

const nativeHeader = "DataColeta;IdNumeric;GrupoProduto;Produto;RazaoSocialPosto;CnpjPosto;Distribuidora;Endereço;Complemento;Bairro;Município;Latitude;Longitude;Uf;RegiaoPolitica;Ensaio;Resultado;UnidadeEnsaio;Conforme\n"
const nativeRow = "2025-01-04;501;synthetic;ETANOL HIDRATADO COMUM;Synthetic station;48.151.623/0001-91;Synthetic;Synthetic street;;Synthetic;Synthetic;;;MT;CENTRO-OESTE;Aspecto;  original text  ;;Não\n"

func nativeArchive(t *testing.T, raw string) (string, prepared.InputIdentity) {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, e := w.Create("2025/PMQC_2025_01.csv")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write([]byte(raw)); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "archive.zip")
	if e = os.WriteFile(p, b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b.Bytes())
	return p, prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(b.Len())}
}
func realRunner(t *testing.T) Runner {
	t.Helper()
	binary := os.Getenv("ANPFUEL_TEST_QUALITY_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_QUALITY_PREPARER_PATH must name the pinned real Rust CLI")
	}
	return Runner{Binary: binary, WorkspaceRoot: t.TempDir()}
}
func TestRealRustArchivePreservesIdentityAndCleanup(t *testing.T) {
	runner := realRunner(t)
	raw := nativeHeader + nativeRow + nativeRow
	p, id := nativeArchive(t, raw)
	result, e := runner.Prepare(context.Background(), Request{Path: p, URL: annualURL, Identity: id, Member: "2025/PMQC_2025_01.csv", Edition: "2025-01"}, unbound)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256([]byte(raw))
	r := result.Resource
	b := result.Batch
	if r.SHA256 != id.SHA256 || r.Bytes != uint64(id.Bytes) || r.MemberSHA256 != hex.EncodeToString(sum[:]) || r.MemberBytes != uint64(len(raw)) || r.Member != "2025/PMQC_2025_01.csv" {
		t.Fatal("resource/member identity lost", r)
	}
	if b.Counts.Input != 2 || b.Counts.Accepted != 1 || b.Counts.Duplicates != 1 || len(b.Assays) != 1 || b.Assays[0].ResultText != "  original text  " || b.Assays[0].Conformity != "NONCONFORMING" || len(b.Links) != 1 || b.Links[0].Active {
		t.Fatal("native loss accounting or isolation", b.Counts)
	}
	if len(result.Periods) != 1 || result.Periods[0].Start != "2025-01-01" || result.Periods[0].End != "2025-01-31" {
		t.Fatal("period", result.Periods)
	}
	children, e := os.ReadDir(runner.WorkspaceRoot)
	if e != nil || len(children) != 0 {
		t.Fatal("success workspace retained", e)
	}
}
func TestRealRustFailureAndResolverOutageAreContained(t *testing.T) {
	runner := realRunner(t)
	for _, raw := range []string{strings.Replace(nativeHeader, "UnidadeEnsaio", "Unknown", 1) + nativeRow, "sample_ref;cnpj;observed_at;fuel;assay_ref;result_text;unit;conformity\n"} {
		p, id := nativeArchive(t, raw)
		calls := 0
		_, e := runner.Prepare(context.Background(), Request{Path: p, URL: annualURL, Identity: id, Member: "2025/PMQC_2025_01.csv", Edition: "2025-01"}, func(context.Context, domain.SourceRecord) (domain.Decision, error) {
			calls++
			return domain.Decision{}, nil
		})
		if e == nil || calls != 0 {
			t.Fatal("unsupported source resolved", e, calls)
		}
	}
	p, id := nativeArchive(t, nativeHeader+nativeRow)
	_, e := runner.Prepare(context.Background(), Request{Path: p, URL: annualURL, Identity: id, Member: "2025/PMQC_2025_01.csv", Edition: "2025-01"}, func(context.Context, domain.SourceRecord) (domain.Decision, error) {
		return domain.Decision{}, errors.New("private database error")
	})
	if e == nil || strings.Contains(e.Error(), "private") {
		t.Fatal("resolution outage leaked or published", e)
	}
	children, e := os.ReadDir(runner.WorkspaceRoot)
	if e != nil || len(children) != 0 {
		t.Fatal("failed workspace retained", e)
	}
}
