package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	source "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	native "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bundledQualityPreparer = "/prepare_quality"
const qualityArchivePrefix = "/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/"

func nativeQualityImport(pool *pgxpool.Pool, binary, tempRoot string) qualityjobs.FileImport {
	identities := directory.NewSourceIdentityReader(pool)
	return qualityjobs.FileImport{Pool: pool, Publish: quality.NewPublisher(pool), Serial: quality.NativeSourceSerial(pool), Fetch: nativeQualityFetch(tempRoot), Prepare: func(ctx context.Context, payload qualityjobs.ImportPayload, file qualityjobs.FileArtifact) (qualityjobs.PreparedBatch, error) {
		type snapshot struct {
			evidence    []domain.IdentityEvidence
			otherBranch bool
		}
		cache := map[string]snapshot{}
		bound := map[string]string{}
		decidedAt := time.Now().UTC().Format("2006-01-02")
		result, e := (native.Runner{Binary: binary, WorkspaceRoot: tempRoot}).Prepare(ctx, native.Request{Path: file.Path, Identity: file.Identity, URL: payload.URL, Member: payload.Member, Edition: payload.Edition}, func(ctx context.Context, record domain.SourceRecord) (domain.Decision, error) {
			known, ok := cache[record.CNPJ]
			if !ok {
				rows, e := identities.Exact(ctx, record.CNPJ)
				if e != nil {
					return domain.Decision{}, e
				}
				for _, row := range rows {
					known.evidence = append(known.evidence, domain.IdentityEvidence{CNPJ: record.CNPJ, StationID: row.StationID, Current: row.Current, StationActive: row.StationActive})
				}
				if len(rows) == 0 {
					branch, e := identities.OtherBranch(ctx, record.CNPJ)
					if e != nil {
						return domain.Decision{}, e
					}
					known.otherBranch = branch
				}
				cache[record.CNPJ] = known
			}
			decision := domain.ResolveCurrentIdentity(record, decidedAt, known.evidence, known.otherBranch)
			if decision.Kind == domain.ResolutionBound {
				bound[record.CNPJ] = decision.Link.StationID
			}
			return decision, nil
		})
		if e != nil {
			return qualityjobs.PreparedBatch{}, errors.New("quality source: native preparation unavailable")
		}
		bindings := make([]directoryapp.SourceBinding, 0, len(bound))
		for cnpj, station := range bound {
			bindings = append(bindings, directoryapp.SourceBinding{CNPJ: cnpj, StationID: station})
		}
		return qualityjobs.PreparedBatch{Assays: result.Batch.Assays, Links: result.Batch.Links, Counts: result.Batch.Counts, ManifestHash: result.Batch.ManifestHash, Periods: result.Periods, MemberSHA256: result.Resource.MemberSHA256, MemberBytes: result.Resource.MemberBytes, Guard: func(ctx context.Context, tx pgx.Tx) error { return identities.Guard(ctx, tx, bindings) }}, nil
	}}
}

// Production HTTPS/TLS/public-IP policy is inherited from the existing owned
// source Fetcher; only the explicit PMQC download origin is added here.
func nativeQualityFetch(tempRoot string) func(context.Context, string) (qualityjobs.FileArtifact, error) {
	return func(ctx context.Context, url string) (qualityjobs.FileArtifact, error) {
		if native.ValidateURL(url) != nil {
			return qualityjobs.FileArtifact{}, errors.New("quality source: download selection refused")
		}
		workspace, e := os.MkdirTemp(tempRoot, "quality-fetch-")
		if e != nil {
			return qualityjobs.FileArtifact{}, errors.New("quality source: download workspace unavailable")
		}
		fetcher := source.NewFetcher([]source.Entry{{ID: "anp-pmqc-native", Host: "www.gov.br", PathPrefix: qualityArchivePrefix, FileGlob: "pmqc_*.zip", Kind: source.KindHistoricalCSV}})
		fetcher.TempDir = workspace
		fetcher.MaxBytes = native.MaxArchiveBytes
		fetcher.MaxRedirects = 3
		result, e := fetcher.Fetch(ctx, url, "", "")
		if e != nil || result.NotModified || result.Bytes <= 0 {
			_ = os.RemoveAll(workspace)
			return qualityjobs.FileArtifact{}, errors.New("quality source: download unavailable")
		}
		return qualityjobs.FileArtifact{Path: result.Path, Identity: qualityInputIdentity(result.SHA256, result.Bytes), Close: func() error {
			first := result.Close()
			second := os.RemoveAll(workspace)
			if first != nil || second != nil {
				return errors.New("quality source: download cleanup unavailable")
			}
			return nil
		}}, nil
	}
}

func qualityInputIdentity(hash string, bytes int64) prepared.InputIdentity {
	return prepared.InputIdentity{SHA256: strings.TrimPrefix(hash, "sha256:"), Bytes: bytes}
}
