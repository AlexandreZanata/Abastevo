package main

import (
	"context"
	"errors"
	"os"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	native "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pmqcListingPage is the verified official datasets page. Its hrefs are the
// only period authority; anchor labels are never trusted.
const pmqcListingPage = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/pmqc-programa-de-monitoramento-da-qualidade-dos-combustiveis"

// nativeQualityLiveDiscovery composes bounded live discovery: the verified
// official listing page through the owned HTTPS Fetcher, exactly one bounded
// archive inspection per run through the same verified fetch and native
// member inventory, and platform enqueue of frozen-provenance imports. The
// handler publishes nothing and schedules nothing by itself.
func nativeQualityLiveDiscovery(pool *pgxpool.Pool) qualityjobs.LiveDiscovery {
	return qualityjobs.LiveDiscovery{
		Pool: pool, Source: "pmqc", Stream: "pmqc-assay-v1",
		List:    fetchPMQCListing,
		Inspect: inspectPMQCArchive,
		Enqueue: qualityImportEnqueue(pool),
	}
}

// fetchPMQCListing downloads the bounded listing page and parses only
// verified archive hrefs. Any unavailable/oversized/unreadable page fails
// the run; an invented inventory is never returned from a failed fetch.
func fetchPMQCListing(ctx context.Context) ([]native.ArchiveRef, error) {
	workspace, err := os.MkdirTemp("", "quality-page-")
	if err != nil {
		return nil, errors.New("quality discovery: listing workspace unavailable")
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	fetcher := source.NewFetcher([]source.Entry{{
		ID: "anp-pmqc-page", Host: "www.gov.br",
		PathPrefix: "/anp/pt-br/centrais-de-conteudo/dados-abertos/", Kind: source.KindListing,
	}})
	fetcher.TempDir = workspace
	fetcher.MaxBytes = native.MaxListingBytes
	fetcher.MaxRedirects = 3
	result, err := fetcher.Fetch(ctx, pmqcListingPage, "", "")
	if err != nil || result.NotModified || result.Bytes <= 0 {
		return nil, errors.New("quality discovery: listing unavailable")
	}
	defer func() { _ = result.Close() }()
	body, err := os.ReadFile(result.Path)
	if err != nil || int64(len(body)) != result.Bytes {
		return nil, errors.New("quality discovery: listing unreadable")
	}
	refs, err := native.ParseListingPage(pmqcListingPage, body)
	if err != nil {
		return nil, errors.New("quality discovery: listing refused")
	}
	return refs, nil
}

// inspectPMQCArchive verifies one downloaded archive through the shared
// bounded fetch policy and enumerates its monthly members; the artifact is
// owned and removed on every path.
func inspectPMQCArchive(ctx context.Context, ref native.ArchiveRef) ([]application.InventoryEntry, error) {
	file, err := nativeQualityFetch("")(ctx, ref.URL)
	if err != nil {
		return nil, err
	}
	if file.Close == nil {
		return nil, errors.New("quality discovery: artifact ownership unavailable")
	}
	defer func() { _ = file.Close() }()
	entries, err := native.Inventory(ctx, file.Path, file.Identity, ref.URL)
	if err != nil {
		return nil, errors.New("quality discovery: archive inspection refused")
	}
	return entries, nil
}
