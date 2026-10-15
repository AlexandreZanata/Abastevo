package main

import (
	"errors"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	qualitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	subscriptiondomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestQualityAuthFailureClassification(t *testing.T) {
	for _, tc := range []struct{ in, want error }{{nil, nil}, {errors.New("synthetic storage outage"), qualitydomain.ErrReportUnavailable}, {pgx.ErrNoRows, qualitydomain.ErrReportUnauthorized}, {accountdomain.ErrCodeUnknown, qualitydomain.ErrReportUnauthorized}, {accountdomain.ErrSessionRevoked, qualitydomain.ErrReportUnauthorized}, {subscriptiondomain.ErrUnauthorized, qualitydomain.ErrReportUnauthorized}, {subscriptiondomain.ErrUnavailable, qualitydomain.ErrReportUnavailable}} {
		if !errors.Is(qualityAuthError(tc.in), tc.want) {
			t.Fatal(tc.in, qualityAuthError(tc.in))
		}
	}
}
