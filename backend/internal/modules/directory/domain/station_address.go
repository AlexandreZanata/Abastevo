package domain

import (
	"strings"
	"unicode"
)

// StationAddress keeps known source components separate from location points.
type StationAddress struct {
	Source     string
	Components map[string]string
}

// ANPStationAddress labels only source-backed address text. A municipality
// code alone cannot stand in for a municipality name.
func ANPStationAddress(components map[string]string) StationAddress {
	source := "municipality"
	if strings.TrimSpace(components["street"]) != "" || strings.TrimSpace(components["raw"]) != "" {
		source = "anp"
	}
	return SelectStationAddress(StationAddress{Source: source, Components: components})
}

// SelectStationAddress applies B-BR-STATION-ADDRESS-01. Caller supplies newest
// evidence first within each source. Returned components cannot mutate input.
func SelectStationAddress(candidates ...StationAddress) StationAddress {
	rank := map[string]int{"anp": 1, "rfb": 2, "osm": 3, "cnefe": 4, "municipality": 5}
	best := StationAddress{}
	for _, candidate := range candidates {
		priority := rank[candidate.Source]
		if priority == 0 || (best.Source != "" && priority >= rank[best.Source]) {
			continue
		}
		if candidate.Source == "municipality" {
			name, uf := strings.TrimSpace(candidate.Components["municipio"]), strings.TrimSpace(candidate.Components["uf"])
			if !strings.ContainsFunc(name, unicode.IsLetter) || len(uf) != 2 || !strings.Contains(" AC AL AM AP BA CE DF ES GO MA MG MS MT PA PB PE PI PR RJ RN RO RR RS SC SE SP TO ", " "+strings.ToUpper(uf)+" ") {
				continue
			}
		} else if strings.TrimSpace(candidate.Components["street"]) == "" && strings.TrimSpace(candidate.Components["raw"]) == "" {
			continue
		}
		components := make(map[string]string, len(candidate.Components))
		for key, value := range candidate.Components {
			components[key] = value
		}
		best = StationAddress{Source: candidate.Source, Components: components}
	}
	return best
}
