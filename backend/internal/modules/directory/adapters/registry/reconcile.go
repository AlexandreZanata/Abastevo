package registry

import (
	"context"
	"errors"
	"fmt"
)

// Canonicalizer publishes staged assertions to stable identities. The
// adapters package implements it over the canonical repository; unit
// tests use a fake. One CNPJ is always one station; different CNPJs
// never merge by address or proximity — succession and duplicate
// consolidation stay audited operator decisions (B-BR-D02), never
// import side effects.
type Canonicalizer interface {
	ResolveStation(ctx context.Context, cnpj, display string, address map[string]string) (string, error)
	RecordReviewedPoint(ctx context.Context, stationID string, lat, lon float64, sourceRef string) error
	SetStatus(ctx context.Context, stationID, status string) error
}

// ReconReport accounts every staged assertion of the run: reconciled
// rows gained a stable UUID (plus explicit status/location
// projections); skipped rows failed canonically and are returned as an
// error so no failure hides inside a green report.
type ReconReport struct {
	RunID      string
	Reconciled int64
	Skipped    int64
}

// stationStatusFor maps staged auth evidence to the station projection.
// Unknown evidence never overwrites: missing data is not a status.
func stationStatusFor(auth string) (string, bool) {
	switch Authorization(auth) {
	case AuthorizationAuthorized:
		return "active", true
	case AuthorizationSuspended:
		return "suspended", true
	case AuthorizationRevoked:
		return "revoked", true
	default:
		return "", false
	}
}

// ReconcileRun publishes one COMPLETE run's assertions to canonical
// identities. Incomplete runs are refused (staging never publishes
// partial snapshots). Only asserted CNPJs are touched: a missing row
// never closes anything, and no address-only merge or invented
// price/location is ever created.
func ReconcileRun(ctx context.Context, store Store, canon Canonicalizer, source, snapshot string) (ReconReport, error) {
	run, err := store.GetRun(ctx, source, snapshot)
	if err != nil {
		return ReconReport{}, err
	}
	if run.State != "complete" {
		return ReconReport{}, fmt.Errorf("registry: run %s is %q, not complete", run.RunID, run.State)
	}
	assertions, err := store.ListAssertions(ctx, run.RunID)
	if err != nil {
		return ReconReport{}, err
	}
	report := ReconReport{RunID: run.RunID}
	var firstErr error
	for _, a := range assertions {
		if err := reconcileOne(ctx, store, canon, a); err != nil {
			report.Skipped++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		report.Reconciled++
	}
	if firstErr != nil {
		return report, firstErr
	}
	return report, nil
}

func reconcileOne(ctx context.Context, store Store, canon Canonicalizer, a Assertion) error {
	stationID, err := canon.ResolveStation(ctx, a.SourceKey, a.DisplayName, a.Address)
	if err != nil {
		return err
	}
	if stationID == "" {
		return errors.New("registry: empty station id")
	}
	if err := store.SetAssertionStation(ctx, a.ID, stationID); err != nil {
		return err
	}
	if status, ok := stationStatusFor(a.AuthState); ok {
		if err := canon.SetStatus(ctx, stationID, status); err != nil {
			return err
		}
	}
	if a.HasCoords && a.LocationQuality == "reviewed" {
		if err := canon.RecordReviewedPoint(ctx, stationID, a.Latitude, a.Longitude, a.SourceReference); err != nil {
			return err
		}
	}
	return nil
}
