package native

import (
	"context"
	"html"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

const EnforcementListingPage = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/acoes-de-fiscalizacao"
const EnforcementResourcePrefix = "/anp/pt-br/centrais-de-conteudo/paineis-dinamicos-da-anp/arquivos-dados-brutos-do-painel-dinamico-da-fiscalizacao-do-abastecimento-da-sfi/"
const EnforcementSourceFile = "dados-fisc-a-partir-2019.xlsx"
const EnforcementResourceURL = "https://www.gov.br" + EnforcementResourcePrefix + EnforcementSourceFile

func ValidateEnforcementSelection(raw, edition string) error {
	if raw != EnforcementResourceURL || edition != application.EnforcementSourceEdition {
		return refused("enforcement source selection")
	}
	return nil
}

// ParseEnforcementListing admits only the reviewed resource href on its exact
// official listing. Labels, query strings and alternate hosts grant no authority.
func ParseEnforcementListing(page string, body []byte) error {
	if page != EnforcementListingPage || len(body) == 0 || len(body) > MaxListingBytes {
		return refused("enforcement listing budget or identity")
	}
	base, _ := url.Parse(page)
	for _, m := range fineHref.FindAllSubmatch(body, -1) {
		href := string(m[1])
		if href == "" {
			href = string(m[2])
		}
		u, e := url.Parse(html.UnescapeString(strings.TrimSpace(href)))
		if e == nil && base.ResolveReference(u).String() == EnforcementResourceURL {
			return nil
		}
	}
	return refused("enforcement approved href unavailable")
}

type EnforcementRunner FineRunner

func (r EnforcementRunner) Prepare(ctx context.Context, req FineRequest) (prepared.EnforcementBatch, error) {
	runner := FineRunner(r)
	if runner.Timeout == 0 {
		runner.Timeout = 2 * time.Minute
	}
	return prepareFileBatch(runner, ctx, req, ValidateEnforcementSelection, prepared.LoadEnforcement, func(snapshot, _, out string) []string { return []string{snapshot, out} }, prepared.EnforcementMaxOutput, true)
}

func InspectEnforcementFile(ctx context.Context, path string, id prepared.InputIdentity) (application.InventoryEntry, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return application.InventoryEntry{}, refused("enforcement inspection file")
	}
	f, e := os.Open(path)
	if e != nil {
		return application.InventoryEntry{}, refused("enforcement inspection unavailable")
	}
	defer f.Close()
	current, e := f.Stat()
	if e != nil || !os.SameFile(info, current) {
		return application.InventoryEntry{}, refused("enforcement inspection replacement")
	}
	if e = verify(ctx, f, id); e != nil {
		return application.InventoryEntry{}, e
	}
	return application.InventoryEntry{URL: EnforcementResourceURL, Label: application.EnforcementSourceEdition, SHA256: id.SHA256, Bytes: uint64(id.Bytes)}, nil
}
