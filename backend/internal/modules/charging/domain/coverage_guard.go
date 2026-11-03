package charging

import (
	"errors"
	"strings"
)

const coverageStates = "AC AL AM AP BA CE DF ES GO MA MG MS MT PA PB PE PI PR RJ RN RO RR RS SC SE SP TO"

type CoverageDrop struct {
	UF       string
	Previous int64
	Next     int64
}

// CoverageDrops reports state regressions strictly greater than 15 percent,
// including a state missing from the candidate. Unassigned sites are tracked
// in the census but cannot offset a regression in a classified state.
func CoverageDrops(previous, next map[string]int64) ([]CoverageDrop, error) {
	states := strings.Fields(coverageStates)
	for _, counts := range []map[string]int64{previous, next} {
		for uf, count := range counts {
			knownState := len(uf) == 2 && strings.Contains(" "+coverageStates+" ", " "+uf+" ")
			if count < 0 || (!knownState && uf != "UNASSIGNED") {
				return nil, errors.New("charging: invalid state census")
			}
		}
	}
	var drops []CoverageDrop
	for _, uf := range states {
		before, after := previous[uf], next[uf]
		// Divide before multiplying to preserve the exact threshold without overflow.
		allowedLoss := before/100*15 + (before%100)*15/100
		if after < before-allowedLoss {
			drops = append(drops, CoverageDrop{UF: uf, Previous: before, Next: after})
		}
	}
	return drops, nil
}
