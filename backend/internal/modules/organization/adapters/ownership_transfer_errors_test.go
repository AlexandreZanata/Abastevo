package adapters

import (
	"context"
	"errors"
	"fmt"
	"testing"

	roles "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
)

// B-BR-EMP-173: a role-period collision during the handover is a conflict,
// never the retryable unavailable outcome.
func TestOwnershipTransferErrorKeepsAssignmentConflict(t *testing.T) {
	for _, err := range []error{roles.ErrAssignmentConflict, fmt.Errorf("handover: %w", roles.ErrAssignmentConflict)} {
		if got := transferError(context.Background(), err); !errors.Is(got, roles.ErrAssignmentConflict) || errors.Is(got, app.ErrUnavailable) {
			t.Fatal("assignment conflict became unavailable", got)
		}
	}
	if got := transferError(context.Background(), errors.New("private storage detail")); !errors.Is(got, app.ErrUnavailable) {
		t.Fatal("unknown storage failure exposed", got)
	}
}
