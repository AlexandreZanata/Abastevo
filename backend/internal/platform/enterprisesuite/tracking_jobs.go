package enterprisesuite

import (
	"errors"

	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

var errTrackingComposition = errors.New("enterprisesuite: incomplete tracking job composition")

// TrackingHandlers composes the track processing job (EMP-GPS-005,
// EDR-298) and retention purge job (EMP-GPS-011) over the ordinary runtime
// pool for the general dispatcher.
func TrackingHandlers(o Options) ([]jobs.Handler, error) {
	if !o.Enabled {
		return nil, nil
	}
	if o.Pool == nil {
		return nil, errTrackingComposition
	}
	processor := &trackingpg.ProcessingStore{Pool: o.Pool, Trips: &trippg.Store{}, Session: &trackingpg.Store{}}
	retention := &trackingpg.RetentionPGStore{Pool: o.Pool}
	return []jobs.Handler{
		trackingpg.ProcessHandler{Processor: processor},
		trackingapp.RetentionHandler{Store: retention},
	}, nil
}
