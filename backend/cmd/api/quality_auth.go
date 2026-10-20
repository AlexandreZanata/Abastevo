package main

import (
	"errors"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	qualitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	subscriptiondomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/jackc/pgx/v5"
)

// Preserve dependency failures as retryable without revealing their internals.
func qualityAuthError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, subscriptiondomain.ErrUnauthorized) {
		return qualitydomain.ErrReportUnauthorized
	}
	switch accountdomain.VerdictCode(err) {
	case "invalid-value", "oidc-unavailable", "key-unavailable":
		return qualitydomain.ErrReportUnavailable
	default:
		return qualitydomain.ErrReportUnauthorized
	}
}
