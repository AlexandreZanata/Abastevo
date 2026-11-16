package enterprisesuite

import (
	"errors"

	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

var errTrackingComposition = errors.New("enterprisesuite: incomplete tracking job composition")

// TrackingHandlers composes the track processing job (EMP-GPS-005,
// EDR-298) over the ordinary runtime pool for the general dispatcher. The
// job derives server-owned summaries only; it never acts for a session.
func TrackingHandlers(o Options) ([]jobs.Handler, error) {
	if !o.Enabled {
		return nil, nil
	}
	if o.Pool == nil {
		return nil, errTrackingComposition
	}
	processor := &trackingpg.ProcessingStore{Pool: o.Pool, Trips: &trippg.Store{}, Session: &trackingpg.Store{}}
	return []jobs.Handler{trackingpg.ProcessHandler{Processor: processor}}, nil
}
