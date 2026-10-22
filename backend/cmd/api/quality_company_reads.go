package main

import (
	"context"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	qualityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	qualitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Only the root translates Directory-owned evidence and transaction guards.
func companyQualityReader(pool *pgxpool.Pool) *qualityread.Reader {
	identity := directory.NewSourceIdentityReader(pool)
	return qualityread.NewReader(pool).WithCompanyIdentity(
		func(ctx context.Context, ids []string) ([]qualitydomain.IdentityEvidence, error) {
			rows, e := identity.Evidence(ctx, ids)
			if e != nil {
				return nil, e
			}
			out := make([]qualitydomain.IdentityEvidence, 0, len(rows))
			for _, r := range rows {
				out = append(out, qualitydomain.IdentityEvidence{CNPJ: r.CNPJ, StationID: r.StationID, Current: r.Current, StationActive: r.StationActive})
			}
			return out, nil
		},
		func(ctx context.Context, tx pgx.Tx, bindings []qualityapp.CompanyBinding) error {
			expected := make([]directoryapp.SourceBinding, 0, len(bindings))
			for _, b := range bindings {
				expected = append(expected, directoryapp.SourceBinding{CNPJ: b.CNPJ, StationID: b.StationID})
			}
			return identity.Guard(ctx, tx, expected)
		})
}
