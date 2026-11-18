package enterprisesuite

import (
	"testing"

	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EDR-298: the processing handler is composed only with the enterprise
// switch and a runtime pool.
func TestTrackingHandlersComposition(t *testing.T) {
	if handlers, err := TrackingHandlers(Options{}); err != nil || handlers != nil {
		t.Fatal("disabled composition", handlers, err)
	}
	if _, err := TrackingHandlers(Options{Enabled: true}); err == nil {
		t.Fatal("composition without a pool")
	}
	pool, err := pgxpool.New(t.Context(), "postgres://synthetic@127.0.0.1:1/synthetic")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	handlers, err := TrackingHandlers(Options{Enabled: true, Pool: pool})
	if err != nil || len(handlers) != 2 || handlers[0].Kind() != trackingapp.ProcessKind || handlers[0].Version() != 1 ||
		handlers[1].Kind() != trackingapp.RetentionKind || handlers[1].Version() != 1 {
		t.Fatal("tracking handlers", err)
	}
}
