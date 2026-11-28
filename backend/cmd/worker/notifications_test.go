package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	notificationsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
	notificationsdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

type recordingNotifier struct{ events []notificationsdomain.Event }

func (r *recordingNotifier) Notify(_ context.Context, _ pgx.Tx, e notificationsdomain.Event) error {
	r.events = append(r.events, e)
	return nil
}

// The composition glue maps every community fact onto the contribution
// payload; fuel is the observation's wire product and reason its first code.
func TestContributionNoticeGlue(t *testing.T) {
	const account = "11111111-1111-4111-8111-111111111111"
	rec := &recordingNotifier{}
	notify := contributionNotices(notificationsapp.Producers{
		Notifier: rec, Clock: time.Now,
		AccountOfContributor: func(_ context.Context, _ pgx.Tx, ref string) (string, bool, error) {
			return account, ref == "tok-bound", nil
		},
	})
	notice := communityapp.DecisionNotice{
		ObservationID: "55555555-5555-4555-8555-555555555555", ContributorRef: "tok-bound",
		StationID: "66666666-6666-4666-8666-666666666666", StationName: "Posto Central",
		Product: "GASOLINE_ADDITIVED", Reason: "evidence-reused",
	}
	if err := notify(context.Background(), nil, notice); err != nil || len(rec.events) != 1 {
		t.Fatalf("notice %v %d", err, len(rec.events))
	}
	want := `{"observation_id":"55555555-5555-4555-8555-555555555555","station_id":"66666666-6666-4666-8666-666666666666","station_name":"Posto Central","fuel":"GASOLINE_ADDITIVED","reason":"evidence-reused"}`
	if got := rec.events[0]; got.AccountID != account || got.Kind != "contribution.rejected" || string(got.Payload) != want {
		t.Fatalf("event %+v %s", got, got.Payload)
	}
	notice.ContributorRef = "tok-device-only"
	if err := notify(context.Background(), nil, notice); err != nil || len(rec.events) != 1 {
		t.Fatalf("contributor without account notified: %v", err)
	}
}
