package prepared

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
)

const nationalInventoryRecords = 205000

var errNationalInventory = errors.New("prepared: national source inventory refused")

// NationalSourceInventory carries evidence from the exact normalization
// decisions linked by the reviewed conflation manifest.
type NationalSourceInventory struct {
	manifestSHA256 string
	catalog        ArtifactDescriptor
	states         map[string]string
}

func VerifyNationalSourceInventory(conflated, normalized fs.FS, manifestSHA256 string) (NationalSourceInventory, error) {
	if conflated == nil || normalized == nil {
		return NationalSourceInventory{}, errNationalInventory
	}
	conflatedRaw, err := readNationalArtifact(conflated, "manifest.json", manifestSHA256, 8<<20)
	if err != nil {
		return NationalSourceInventory{}, err
	}
	var manifest struct {
		Format      string                        `json:"format"`
		InputSHA256 string                        `json:"input_manifest_sha256"`
		Files       map[string]ArtifactDescriptor `json:"files"`
	}
	if err := json.Unmarshal(conflatedRaw, &manifest); err != nil || manifest.Format != "abastevo.ev-conflated-v1" {
		return NationalSourceInventory{}, errNationalInventory
	}
	catalog := manifest.Files["catalog.ndjson"]
	if !validHash(catalog.SHA256) || catalog.Bytes < 1 || catalog.Bytes > offerLimit ||
		catalog.Records < 1 || catalog.Records > nationalInventoryRecords {
		return NationalSourceInventory{}, errNationalInventory
	}
	normalRaw, err := readNationalArtifact(normalized, "manifest.json", manifest.InputSHA256, 8<<20)
	if err != nil {
		return NationalSourceInventory{}, err
	}
	var normalization struct {
		Format string                        `json:"format"`
		Files  map[string]ArtifactDescriptor `json:"files"`
		Pins   struct {
			OCMRows int `json:"ocm_rows"`
		} `json:"source_pins"`
		Summary struct {
			AcceptedSites int `json:"accepted_source_sites"`
			Equipment     int `json:"source_equipment"`
			OSMCandidates int `json:"osm_candidates"`
			Quarantined   int `json:"quarantined"`
			Ignored       int `json:"ignored"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(normalRaw, &normalization); err != nil || normalization.Format != "abastevo.ev-source-sites-v1" {
		return NationalSourceInventory{}, errNationalInventory
	}
	descriptor := normalization.Files["decisions.ndjson"]
	if descriptor.Bytes < 1 || descriptor.Bytes > 64<<20 ||
		descriptor.Records < 1 || descriptor.Records > nationalInventoryRecords {
		return NationalSourceInventory{}, errNationalInventory
	}
	raw, err := readNationalArtifact(normalized, "decisions.ndjson", descriptor.SHA256, descriptor.Bytes)
	if err != nil {
		return NationalSourceInventory{}, err
	}
	if int64(len(raw)) != descriptor.Bytes || raw[len(raw)-1] != '\n' {
		return NationalSourceInventory{}, errNationalInventory
	}
	states, sources, totals, err := decodeNationalDecisions(raw, int(descriptor.Records))
	summary := normalization.Summary
	if err != nil || sources["ocm"] != normalization.Pins.OCMRows || sources["osm"] != summary.OSMCandidates ||
		summary.AcceptedSites < 1 || summary.AcceptedSites > nationalInventoryRecords ||
		summary.Equipment < 0 || summary.Equipment > nationalInventoryRecords ||
		totals["ACCEPTED"] != summary.AcceptedSites+summary.Equipment ||
		totals["QUARANTINED"] != summary.Quarantined || totals["IGNORED"] != summary.Ignored {
		return NationalSourceInventory{}, errNationalInventory
	}
	return NationalSourceInventory{manifestSHA256, catalog, states}, nil
}

func decodeNationalDecisions(raw []byte, count int) (map[string]string, map[string]int, map[string]int, error) {
	states := make(map[string]string, count)
	sources, totals := map[string]int{}, map[string]int{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024), 4096)
	for scanner.Scan() {
		var decision struct {
			Key   string `json:"key"`
			State string `json:"state"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &decision); err != nil {
			return nil, nil, nil, errNationalInventory
		}
		namespace, valid := catalogSourceNamespace(decision.Key)
		if !valid || states[decision.Key] != "" || len(states) == count {
			return nil, nil, nil, errNationalInventory
		}
		switch decision.State {
		case "ACCEPTED", "QUARANTINED", "IGNORED":
		default:
			return nil, nil, nil, errNationalInventory
		}
		states[decision.Key] = decision.State
		sources[namespace]++
		totals[decision.State]++
	}
	if scanner.Err() != nil || len(states) != count {
		return nil, nil, nil, errNationalInventory
	}
	return states, sources, totals, nil
}
