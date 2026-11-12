package adapters_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func validEnvelope() domain.Envelope {
	return domain.Envelope{Version: 1, OrgID: "7b0e2a54-0000-4000-8000-000000000001", Consumer: "projection.refresh", Event: domain.Event{ID: "7b0e2a54-0000-4000-8000-000000000002", Name: "example.record.changed", TargetType: "example.record", TargetID: "7b0e2a54-0000-4000-8000-000000000003", Revision: 1}}
}
func TestEnvelopeRejectsAmbiguousAndSensitivePayload(t *testing.T) {
	good := validEnvelope()
	payload := string(good.Payload())
	if decoded, err := adapters.DecodePayload(good.Payload()); err != nil || decoded != good {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "null", strings.Repeat(" ", domain.MaxPayloadBytes+1),
		strings.Replace(payload, `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(payload, `"org_id"`, `"Org_id"`, 1),
		strings.Replace(payload, `"revision":1`, `"revision":null`, 1),
		strings.Replace(payload, `"revision":1`, `"revision":1.5`, 1),
		strings.TrimSuffix(payload, "}") + `,"token":"PRIVATE-CREDENTIAL"}`,
		payload + "{}",
	} {
		if _, err := adapters.DecodePayload([]byte(raw)); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("unsafe payload accepted", err)
		}
	}
	for _, value := range []any{good.Event, good} {
		var log bytes.Buffer
		slog.New(slog.NewJSONHandler(&log, nil)).Info("event", "value", value)
		output := fmt.Sprintf("%v %+v %#v", value, value, value) + log.String()
		if strings.Contains(output, good.OrgID) || strings.Contains(output, good.TargetID) {
			t.Fatal("private references logged")
		}
	}
	var h *adapters.Handler
	if !errors.Is(h.Handle(t.Context(), jobs.Job{}), domain.ErrUnavailable) {
		t.Fatal("unwired handler accepted")
	}
	if _, err := adapters.NewHandler(adapters.NewStore(nil), domain.Catalog{}, nil); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := adapters.NewStore(nil).Enqueue(t.Context(), kernel.OrgScope{}, nil, good, ""); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
}
