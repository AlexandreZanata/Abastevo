package main

import (
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	companyhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

func newCompanyHandler(cfg config.Config, pool *pgxpool.Pool, accounts *account.Service) (*companyhttp.Handler, error) {
	if !cfg.EnterpriseEnabled {
		return nil, nil
	}
	service := &company.Service{Enabled: true, Store: &adapters.Store{Pool: pool}, Hasher: accountdomain.SHA256Hasher{},
		Prove: adapters.AccountProof(accounts), Now: time.Now, NewID: newUUID, NewSecret: accountdomain.GenerateToken}
	handler, err := companyhttp.New(service, "https://"+cfg.CanonicalHost)
	if err != nil {
		return nil, err
	}
	return &handler, nil
}
