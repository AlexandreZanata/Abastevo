package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
)

// PreparedPublicationPorts keeps Directory and StationProfile ownership explicit.
// Composition roots supply these ports; registry never imports either adapter.
type PreparedPublicationPorts struct {
	Canonical     Canonicalizer
	Locality      func(context.Context, string, string, string) error
	EnsureProfile func(context.Context, string) error
}

// PublishPreparedRegistry publishes only the bound complete registry input, in
// pages of at most 100. PMQC input/candidates are never publication inputs.
// Restart reuses canonical IDs and profiles. No authorization/badge is inferred.
func PublishPreparedRegistry(ctx context.Context, store *PGStore, ports PreparedPublicationPorts, raw []byte) (ReconReport, error) {
	if len(raw) > preparedMaxManifest {
		return ReconReport{}, fmt.Errorf("registry: oversized publication manifest")
	}
	manifest, err := validateBatchManifest(raw)
	if err != nil {
		return ReconReport{}, err
	}
	if ports.Canonical == nil || ports.Locality == nil || ports.EnsureProfile == nil {
		return ReconReport{}, fmt.Errorf("registry: publication ports missing")
	}
	found := false
	for _, input := range manifest.Inputs {
		if input.Key == "registry-13col" {
			found = true
		}
	}
	if !found {
		return ReconReport{}, fmt.Errorf("registry: no registry input to publish")
	}
	run, err := store.GetRun(ctx, SourcePrep, "station-prep:"+manifest.RunID+":registry-13col")
	if err != nil {
		return ReconReport{}, err
	}
	if run.State != "complete" {
		return ReconReport{}, fmt.Errorf("registry: publication requires complete input")
	}
	uid, err := mustUUID(run.RunID)
	if err != nil {
		return ReconReport{}, err
	}
	binding, err := store.Q.GetPreparedRunBinding(ctx, uid)
	if err != nil {
		return ReconReport{}, err
	}
	sum := sha256.Sum256(raw)
	if binding != hex.EncodeToString(sum[:]) {
		return ReconReport{}, fmt.Errorf("registry: publication manifest binding mismatch")
	}
	report := ReconReport{RunID: run.RunID}
	after, _ := mustUUID("00000000-0000-0000-0000-000000000000")
	for {
		page, err := store.Q.ListPreparedRegistryPage(ctx, directory.ListPreparedRegistryPageParams{RunID: uid, AfterID: after})
		if err != nil {
			return report, err
		}
		if len(page) == 0 {
			break
		}
		for _, row := range page {
			address := map[string]string{}
			if err := json.Unmarshal(row.Address, &address); err != nil {
				return report, err
			}
			stationID, err := ports.Canonical.ResolveStation(ctx, row.SourceKey, row.DisplayName, address)
			if err != nil {
				return report, err
			}
			if err := ports.Locality(ctx, stationID, row.MunicipalityCode.String, row.State.String); err != nil {
				return report, err
			}
			if err := ports.EnsureProfile(ctx, stationID); err != nil {
				return report, err
			}
			if err := store.SetAssertionStation(ctx, uuidString(row.ID), stationID); err != nil {
				return report, err
			}
			report.Reconciled++
			after = row.ID
		}
	}
	want, err := store.Q.CountPreparedPublicationRows(ctx, uid)
	if err != nil {
		return report, err
	}
	if report.Reconciled != want {
		return report, fmt.Errorf("registry: publication count %d differs from complete registry input %d", report.Reconciled, want)
	}
	return report, nil
}
