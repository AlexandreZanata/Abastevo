package adapters

import (
	"context"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

func TestStorePreservesOnlySafeCurrentAuthorityRefusals(t *testing.T) {
	for _, expected := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrFreshAuthRequired, access.ErrEntitlementRequired, access.ErrUnauthenticated, permission.ErrPermissionDenied, permission.ErrResourceNotFound} {
		t.Run(expected.Error(), func(t *testing.T) {
			if got := storeError(context.Background(), expected); !errors.Is(got, expected) {
				t.Fatal("current source denial became unavailability")
			}
		})
	}
	for _, err := range []error{errors.New("synthetic private provider secret"), &pgconn.PgError{Code: "XX000", Message: "synthetic private record"}, &pgconn.PgError{Code: "23505", ConstraintName: "unregistered_private"}} {
		if got := storeError(context.Background(), err); got != app.ErrUnavailable {
			t.Fatal("unregistered provider details escaped")
		}
	}
}
