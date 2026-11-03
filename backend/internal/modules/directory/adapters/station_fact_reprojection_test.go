package adapters

import (
	"context"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"testing"
)

func TestStationFactReprojectionRejectsInvalidEnvelopeBeforeIO(t *testing.T) {
	handler := StationFactReprojection{}
	for _, raw := range []string{
		`{"version":2,"campaign_id":"11111111-1111-4111-8111-111111111111"}`,
		`{"version":1,"campaign_id":"invalid"}`,
		`{"version":1,"campaign_id":"11111111-1111-4111-8111-111111111111","unknown":"synthetic"}`,
		`{"version":1,"campaign_id":"11111111-1111-4111-8111-111111111111","station_id":"invalid"}`,
		`{"version":1,"campaign_id":"11111111-1111-4111-8111-111111111111","station_id":"22222222-2222-4222-8222-222222222222","after_id":"33333333-3333-4333-8333-333333333333"}`,
	} {
		if err := handler.Handle(context.Background(), platformjobs.Job{Payload: []byte(raw)}); err == nil {
			t.Fatal("invalid campaign job accepted", raw)
		}
	}
}
