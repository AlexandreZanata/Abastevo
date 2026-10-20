//go:build integration

package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRustQualityPreparerRoundTrip(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_QUALITY_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_QUALITY_PREPARER_PATH must name the built trusted Rust quality CLI")
	}
	_, file, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(file), "../../../../../../contracts/testdata/station-prep/quality-assay-sample.csv")
	raw, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	dir := filepath.Join(t.TempDir(), "prepared")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if e = exec.CommandContext(ctx, binary, source, dir).Run(); e != nil {
		t.Fatal("trusted preparer failed", e)
	}
	manifest, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	b, e := Load(ctx, manifest, func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(dir, name)) }, InputIdentity{Bytes: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])}, unbound)
	if e != nil || b.Counts.Input != 12 || b.Counts.Accepted != 7 || b.Counts.Input != b.Counts.Accepted+b.Counts.Duplicates+b.Counts.Quarantined {
		t.Fatal("actual Rust contract mismatch", b.Counts, e)
	}
	for _, a := range b.Assays {
		if strings.Contains(a.SampleRef, a.SourceCNPJ) {
			t.Fatal("public sample leaked CNPJ")
		}
	}
	// Native-layout synthetic source proves empty fields and literal Unicode
	// comparators survive the real Rust encoder, not Go's test serialization.
	native := filepath.Join(t.TempDir(), "native.csv")
	text := "DataColeta;IdNumeric;GrupoProduto;Produto;RazaoSocialPosto;CnpjPosto;Distribuidora;Endereço;Complemento;Bairro;Município;Latitude;Longitude;Uf;RegiaoPolitica;Ensaio;Resultado;UnidadeEnsaio;Conforme\n" +
		"2026-09-04;501;synthetic;ETANOL HIDRATADO COMUM;Synthetic;04.218.406/0001-04;Synthetic;Synthetic street;;Synthetic;Synthetic;;;MT;CENTRO-OESTE;Aspecto;;;Não\n" +
		"2026-09-04;501;synthetic;ETANOL HIDRATADO COMUM;Synthetic;04.218.406/0001-04;Synthetic;Synthetic street;;Synthetic;Synthetic;;;MT;CENTRO-OESTE;Outro;<1,0 Límpido\u2028Ω;%;Sim\n"
	if e = os.WriteFile(native, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
	dir = filepath.Join(t.TempDir(), "native-prepared")
	if e = exec.CommandContext(ctx, binary, native, dir).Run(); e != nil {
		t.Fatal("native preparer failed", e)
	}
	manifest, e = os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	sum = sha256.Sum256([]byte(text))
	b, e = Load(ctx, manifest, func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(dir, name)) }, InputIdentity{Bytes: int64(len(text)), SHA256: hex.EncodeToString(sum[:])}, unbound)
	if e != nil || len(b.Assays) != 2 || len(b.Links) != 1 || b.Assays[0].ResultText != "" || b.Assays[1].ResultText != "<1,0 Límpido\u2028Ω" {
		t.Fatal("native lossless round trip", len(b.Assays), len(b.Links), e)
	}
}
