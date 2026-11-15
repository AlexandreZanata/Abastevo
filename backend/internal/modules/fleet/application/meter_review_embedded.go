package application

import (
	"context"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
)

// ReviewedEmbeddedReading is the embedded reading of an accepted held field
// fact of another owner (EMP-TRIP-007, EDR-264): the original operation's
// author membership and enrolled device, its original receipt and the
// resolution that accepted it.
type ReviewedEmbeddedReading struct {
	private
	ResolutionID, AuthorMembershipID, EnrollmentID string
	ReceivedAt                                     time.Time
	Command                                        MeterReadingCommand
}

// ReviewedReadingRepository stores such a reading under its resolution, at
// its original receipt and classified there (B-BR-EMP-283), inside the
// resolving transaction that already holds the organization root and asset
// locks. It proves no authority of its own: the database guard re-proves the
// accepted resolution and the held fact.
type ReviewedReadingRepository interface {
	ApplyReviewedEmbeddedTx(context.Context, syncapp.DBTX, kernel.OrgScope, ReviewedEmbeddedReading) (meter.Reading, error)
}
