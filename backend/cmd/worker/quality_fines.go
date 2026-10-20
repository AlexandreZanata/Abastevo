package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bundledFinePreparer = "/prepare_sanctions"
const bundledFineHistoryPreparer = "/prepare_fine_history"

func nativeFineImport(pool *pgxpool.Pool, binary, historyBinary, root string) qualityjobs.FineFileImport {
	identities := directory.NewSourceIdentityReader(pool)
	publisher := quality.NewPublisher(pool).WithCompanyGuard(func(ctx context.Context, tx pgx.Tx, bindings []application.CompanyBinding) error {
		expected := make([]directoryapp.SourceBinding, 0, len(bindings))
		for _, b := range bindings {
			expected = append(expected, directoryapp.SourceBinding{CNPJ: b.CNPJ, StationID: b.StationID})
		}
		return identities.Guard(ctx, tx, expected)
	})
	return qualityjobs.FineFileImport{Pool: pool, Publish: publisher, Associate: func(ctx context.Context, records []domain.CompanyObservationRecord) (*application.CompanyAssociationInput, error) {
		day := time.Now().UTC().Format("2006-01-02")
		decisions, e := application.PlanCompanyAssociations(ctx, records, day, func(ctx context.Context, cnpjs []string) (application.CompanyIdentitySnapshot, error) {
			rows, e := identities.Evidence(ctx, cnpjs)
			if e != nil {
				return application.CompanyIdentitySnapshot{}, e
			}
			out := application.CompanyIdentitySnapshot{}
			known := make(map[string]bool, len(cnpjs))
			for _, row := range rows {
				known[row.CNPJ] = true
				out.Evidence = append(out.Evidence, domain.IdentityEvidence{CNPJ: row.CNPJ, StationID: row.StationID, Current: row.Current, StationActive: row.StationActive})
			}
			missing := make([]string, 0)
			for _, cnpj := range cnpjs {
				if !known[cnpj] {
					missing = append(missing, cnpj)
				}
			}
			out.OtherBranches, e = identities.OtherBranches(ctx, missing)
			return out, e
		})
		if e != nil {
			return nil, e
		}
		return &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: day, Decisions: decisions}, nil
	}, Serial: quality.NativeSourceSerial(pool), Fetch: nativeFineFetch(root), Prepare: func(ctx context.Context, payload qualityjobs.ImportPayload, file qualityjobs.FileArtifact) (prepared.FineBatch, error) {
		return (native.FineRunner{Binary: binary, WorkspaceRoot: root}).Prepare(ctx, native.FineRequest{Path: file.Path, URL: payload.URL, Edition: payload.Edition, Identity: file.Identity})
	}, PrepareHistory: func(ctx context.Context, payload qualityjobs.ImportPayload, file qualityjobs.FileArtifact) (prepared.FineHistoryBatch, error) {
		return (native.FineHistoryRunner{Binary: historyBinary, WorkspaceRoot: root}).Prepare(ctx, native.FineRequest{Path: file.Path, URL: payload.URL, Edition: payload.Edition, Identity: file.Identity})
	}}
}

// The existing Fetcher owns TLS, public-IP pinning, redirect validation,
// private files, deadlines and the streamed content digest. No job can select
// a host, local path, executable or alternate fetch security policy.
func nativeFineFetch(root string) func(context.Context, string) (qualityjobs.FileArtifact, error) {
	return func(ctx context.Context, raw string) (qualityjobs.FileArtifact, error) {
		if _, e := native.FineResourcePeriod(raw); e != nil {
			return qualityjobs.FileArtifact{}, errors.New("fine source: resource selection refused")
		}
		workspace, e := os.MkdirTemp(root, "quality-fine-fetch-")
		if e != nil {
			return qualityjobs.FileArtifact{}, errors.New("fine source: workspace unavailable")
		}
		fetcher := source.NewFetcher([]source.Entry{{ID: "anp-fine-native", Host: "www.gov.br", PathPrefix: native.FineResourcePrefix, FileGlob: "multas*", Kind: source.KindHistoricalCSV}})
		fetcher.TempDir, fetcher.MaxBytes, fetcher.MaxRedirects = workspace, prepared.MaxBytes, 3
		result, e := fetcher.Fetch(ctx, raw, "", "")
		if e != nil || result.NotModified || result.Bytes <= 0 {
			_ = os.RemoveAll(workspace)
			return qualityjobs.FileArtifact{}, errors.New("fine source: download unavailable")
		}
		return qualityjobs.FileArtifact{Path: result.Path, Identity: qualityInputIdentity(result.SHA256, result.Bytes), Close: func() error {
			first := result.Close()
			second := os.RemoveAll(workspace)
			if first != nil || second != nil {
				return errors.New("fine source: download cleanup unavailable")
			}
			return nil
		}}, nil
	}
}

func nativeFineLiveDiscovery(pool *pgxpool.Pool) qualityjobs.FineDiscovery {
	return qualityjobs.FineDiscovery{Pool: pool, Serial: quality.NativeSourceSerial(pool), List: fetchFineListing, Inspect: inspectFineResource, Enqueue: func(ctx context.Context, payload qualityjobs.ImportPayload, dedupe string) (platformjobs.EnqueueResult, error) {
		raw, e := json.Marshal(payload)
		if e != nil {
			return platformjobs.EnqueueResult{}, e
		}
		return platformjobs.Enqueue(ctx, pool, qualityjobs.NativeFineImportKind, raw, dedupe, 3, time.Time{})
	}}
}

func fetchFineListing(ctx context.Context) (refs []native.FineRef, err error) {
	workspace, e := os.MkdirTemp("", "quality-fine-page-")
	if e != nil {
		return nil, errors.New("fine discovery: workspace unavailable")
	}
	defer func() {
		if e := os.RemoveAll(workspace); e != nil {
			refs, err = nil, errors.New("fine discovery: workspace cleanup unavailable")
		}
	}()
	fetcher := source.NewFetcher([]source.Entry{{ID: "anp-fine-listing", Host: "www.gov.br", PathPrefix: "/anp/pt-br/centrais-de-conteudo/dados-abertos/", Kind: source.KindListing}})
	fetcher.TempDir, fetcher.MaxBytes, fetcher.MaxRedirects = workspace, native.MaxListingBytes, 3
	result, e := fetcher.Fetch(ctx, native.FineListingPage, "", "")
	if e != nil || result.NotModified || result.Bytes <= 0 {
		return nil, errors.New("fine discovery: listing unavailable")
	}
	defer func() {
		if e := result.Close(); e != nil {
			refs, err = nil, errors.New("fine discovery: listing cleanup unavailable")
		}
	}()
	body, e := os.ReadFile(result.Path)
	if e != nil || int64(len(body)) != result.Bytes {
		return nil, errors.New("fine discovery: listing unreadable")
	}
	return native.ParseFineListingPage(native.FineListingPage, body)
}

func inspectFineResource(ctx context.Context, ref native.FineRef) (entry application.InventoryEntry, err error) {
	if native.ValidateFineResourceSelection(ref.URL, ref.Period) != nil {
		return entry, errors.New("fine discovery: selection refused")
	}
	file, e := nativeFineFetch("")(ctx, ref.URL)
	if e != nil {
		return entry, e
	}
	defer func() {
		if file.Close == nil || file.Close() != nil {
			entry, err = application.InventoryEntry{}, errors.New("fine discovery: resource cleanup unavailable")
		}
	}()
	return native.InspectFineFile(ctx, file.Path, ref, file.Identity)
}
