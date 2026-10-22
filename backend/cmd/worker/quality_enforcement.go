package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bundledEnforcementPreparer = "/prepare_enforcement"

func nativeEnforcementFetch(root string) func(context.Context, string) (qualityjobs.FileArtifact, error) {
	return nativeSourceFileFetch(root, func(raw string) error {
		return native.ValidateEnforcementSelection(raw, application.EnforcementSourceEdition)
	}, source.Entry{ID: "anp-enforcement-workbook", Host: "www.gov.br", PathPrefix: native.EnforcementResourcePrefix, FileGlob: native.EnforcementSourceFile, Kind: "enforcement-workbook"})
}
func nativeEnforcementImport(pool *pgxpool.Pool, binary, root string) qualityjobs.EnforcementFileImport {
	return qualityjobs.EnforcementFileImport{Pool: pool, Publish: quality.NewPublisher(pool), Fetch: nativeEnforcementFetch(root), Serial: quality.NativeSourceSerial(pool), Prepare: func(ctx context.Context, p qualityjobs.ImportPayload, f qualityjobs.FileArtifact) (prepared.EnforcementBatch, error) {
		return (native.EnforcementRunner{Binary: binary, WorkspaceRoot: root}).Prepare(ctx, native.FineRequest{Path: f.Path, URL: p.URL, Edition: p.Edition, Identity: f.Identity})
	}}
}
func nativeEnforcementLiveDiscovery(pool *pgxpool.Pool) qualityjobs.EnforcementDiscovery {
	return qualityjobs.EnforcementDiscovery{Pool: pool, Serial: quality.NativeSourceSerial(pool), List: fetchEnforcementListing, Inspect: func(ctx context.Context) (entry application.InventoryEntry, err error) {
		f, e := nativeEnforcementFetch("")(ctx, native.EnforcementResourceURL)
		if e != nil {
			return entry, e
		}
		defer func() {
			if f.Close == nil || f.Close() != nil {
				entry = application.InventoryEntry{}
				err = errors.New("enforcement discovery: cleanup unavailable")
			}
		}()
		return native.InspectEnforcementFile(ctx, f.Path, f.Identity)
	}, Enqueue: func(ctx context.Context, p qualityjobs.ImportPayload, dedupe string) (platformjobs.EnqueueResult, error) {
		raw, e := json.Marshal(p)
		if e != nil {
			return platformjobs.EnqueueResult{}, e
		}
		return platformjobs.Enqueue(ctx, pool, qualityjobs.EnforcementImportKind, raw, dedupe, 3, time.Time{})
	}}
}
func fetchEnforcementListing(ctx context.Context) (err error) {
	workspace, e := os.MkdirTemp("", "quality-enforcement-listing-")
	if e != nil {
		return errors.New("enforcement discovery: workspace unavailable")
	}
	defer func() {
		if os.RemoveAll(workspace) != nil {
			err = errors.New("enforcement discovery: cleanup unavailable")
		}
	}()
	fetcher := source.NewFetcher([]source.Entry{{ID: "anp-enforcement-listing", Host: "www.gov.br", PathPrefix: "/anp/pt-br/centrais-de-conteudo/dados-abertos/", Kind: source.KindListing}})
	fetcher.TempDir, fetcher.MaxBytes, fetcher.MaxRedirects = workspace, native.MaxListingBytes, 3
	result, e := fetcher.Fetch(ctx, native.EnforcementListingPage, "", "")
	if e != nil || result.NotModified || result.Bytes <= 0 {
		return errors.New("enforcement discovery: listing unavailable")
	}
	defer func() {
		if result.Close() != nil {
			err = errors.New("enforcement discovery: listing cleanup unavailable")
		}
	}()
	raw, e := os.ReadFile(result.Path)
	if e != nil || int64(len(raw)) != result.Bytes {
		return errors.New("enforcement discovery: listing unreadable")
	}
	return native.ParseEnforcementListing(native.EnforcementListingPage, raw)
}
