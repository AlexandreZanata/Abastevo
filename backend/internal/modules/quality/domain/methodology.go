package domain

import (
	"sort"
	"strings"
)

// MethodologyVersion identifies the frozen sample-judgment method. The
// 2026-10 method mirrors the official ANP PMQC open-data guidance: the file
// unit is the assay, the judgment unit is the sample, and the Aspecto
// exception is fuel-specific. Version bumps are required before any change.
const MethodologyVersion = "pmqc-sample-judgment-v2026-10a"

// MethodologyLimitation is the frozen calibration boundary: dimensions are
// exposed, but no numeric weights or sufficiency thresholds exist yet, so
// every score state stays UNSCORED until FQ-12 calibration is accepted.
const MethodologyLimitation = "dimensions only: no numeric weights or sufficiency thresholds are frozen; score stays UNSCORED"

// Judgment reason codes. They are part of the frozen contract.
const (
	ReasonEthanolAspectoCleared = "ethanol_aspecto_complementary_conforming"
	ReasonDieselAspectoCleared  = "diesel_aspecto_complementary_conforming"
	ReasonAspectoUnconfirmed    = "aspecto_vice_unconfirmed"
	ReasonDefectUnclassified    = "diesel_defect_unclassified"
	ReasonDieselRuleEpoch       = "diesel_rule_epoch_unverified"
	ReasonReplicaConflict       = "replica_conflict"
	ReasonUnknownAssay          = "unknown_assay_status"
)

// SampleVerdict is the judged quality of one sample under the methodology.
type SampleVerdict string

const (
	SampleConforming    SampleVerdict = "CONFORMING"
	SampleNonconforming SampleVerdict = "NONCONFORMING"
	// SampleUnresolved means the verified rules cannot conclude; it is never
	// converted into a failure or a pass.
	SampleUnresolved SampleVerdict = "UNRESOLVED"
)

// AssayObservation is one assay row after replica collapsing: the native
// assay name, its verbatim result and the closed ledger conformity.
type AssayObservation struct {
	Ensaio     string
	Result     string
	Conformity Conformity
}

// SampleFacts is one distinct source sample (visit) of one fuel. SampleKey is
// the frozen source sample identity; StationID and ObservedOn are dimensions,
// never validity windows.
type SampleFacts struct {
	SampleKey  string
	SourceCNPJ string
	StationID  string
	ObservedOn string
	Fuel       Fuel
	Judgment   SampleJudgment
}

// SampleJudgment is the frozen verdict plus its reasons and the distinct
// nonconforming assay names (canonical order).
type SampleJudgment struct {
	Verdict             SampleVerdict
	Reasons             []string
	NonconformingAssays []string
}

// frozen assay names from the official vocabulary (2025/2026 open files).
const (
	assayAspecto = "Aspecto"
)

var ethanolComplementaries = []string{"Resíduo por Evaporação", "Material Não Volátil", "Teor de Material Não Volátil"}

// JudgeSample implements the official aggregation: any non-Aspecto failure
// is a sample failure; a sole Aspecto failure for ethanol or diesel is
// resolved through the complementary assays of its fuel; gasoline has no
// Aspecto exception; and cases the verified rules cannot decide stay
// UNRESOLVED instead of being guessed.
func JudgeSample(facts SampleFacts, assays []AssayObservation) SampleJudgment {
	distinct, conflicts := collapseReplicas(assays)
	if conflicts {
		return SampleJudgment{Verdict: SampleUnresolved, Reasons: []string{ReasonReplicaConflict}}
	}
	if len(distinct) == 0 {
		return SampleJudgment{Verdict: SampleUnresolved, Reasons: []string{ReasonUnknownAssay}}
	}
	nonconforming := make([]string, 0, 1)
	unknown := false
	var aspecto *AssayObservation
	for i := range distinct {
		row := distinct[i]
		switch row.Conformity {
		case Nonconforming:
			nonconforming = append(nonconforming, row.Ensaio)
		case Conforming:
		default:
			unknown = true
		}
		if row.Ensaio == assayAspecto {
			aspecto = &distinct[i]
		}
	}
	sort.Strings(nonconforming)
	out := SampleJudgment{NonconformingAssays: nonconforming}
	if unknown {
		return SampleJudgment{Verdict: SampleUnresolved, Reasons: []string{ReasonUnknownAssay}}
	}
	aspectFailed := aspecto != nil && aspecto.Conformity == Nonconforming
	othersFailed := false
	for _, name := range nonconforming {
		if name != assayAspecto {
			othersFailed = true
			break
		}
	}
	if othersFailed {
		out.Verdict = SampleNonconforming
		return out
	}
	if !aspectFailed {
		out.Verdict = SampleConforming
		return out
	}
	switch fuelFamily(facts.Fuel) {
	case familyGasoline:
		out.Verdict = SampleNonconforming
		return out
	case familyEthanol:
		return judgeEthanolAspecto(distinct, out)
	case familyDiesel:
		return judgeDieselAspecto(facts, distinct, aspecto.Result, out)
	default:
		out.Verdict = SampleUnresolved
		out.Reasons = []string{ReasonDefectUnclassified}
		return out
	}
}

type fuelFamilyKind string

const (
	familyGasoline fuelFamilyKind = "gasoline"
	familyEthanol  fuelFamilyKind = "ethanol"
	familyDiesel   fuelFamilyKind = "diesel"
	familyOther    fuelFamilyKind = "other"
)

func fuelFamily(fuel Fuel) fuelFamilyKind {
	switch fuel {
	case FuelEthanol:
		return familyEthanol
	case FuelGasolineRegular, FuelGasolineAdditived, FuelGasolinePremiumGrade:
		return familyGasoline
	case FuelDieselS500, FuelDieselS10:
		return familyDiesel
	default:
		return familyOther
	}
}

// collapseReplicas reduces repeated rows of one assay. Replicas must agree:
// mixed conforming/nonconforming rows of the same assay are a conflict, not
// evidence, and repeated unknown rows collapse to unknown.
func collapseReplicas(assays []AssayObservation) ([]AssayObservation, bool) {
	byName := map[string][]AssayObservation{}
	order := []string{}
	for _, row := range assays {
		name := strings.TrimSpace(row.Ensaio)
		if name == "" {
			continue
		}
		if _, seen := byName[name]; !seen {
			order = append(order, name)
		}
		byName[name] = append(byName[name], AssayObservation{Ensaio: name, Result: row.Result, Conformity: row.Conformity})
	}
	sort.Strings(order)
	out := make([]AssayObservation, 0, len(order))
	for _, name := range order {
		rows := byName[name]
		status := rows[0].Conformity
		conflict := false
		for _, row := range rows[1:] {
			if row.Conformity != status {
				conflict = true
			}
		}
		if conflict {
			return nil, true
		}
		out = append(out, rows[0])
	}
	return out, false
}

// judgeEthanolAspecto applies RANP nº 907/2022: the Aspecto fails only when
// the evaporation residue (or its non-volatile material substitute) is
// nonconforming; absent complementaries leave the vice unconfirmed, which
// the official guidance treats as conforming for judgment.
func judgeEthanolAspecto(distinct []AssayObservation, out SampleJudgment) SampleJudgment {
	cleared, confirmed, present := complementaryOutcome(distinct, ethanolComplementaries)
	switch {
	case confirmed:
		out.Verdict = SampleNonconforming
	case cleared:
		out.Verdict = SampleConforming
		out.Reasons = []string{ReasonEthanolAspectoCleared}
	case !present:
		out.Verdict = SampleConforming
		out.Reasons = []string{ReasonAspectoUnconfirmed}
	default:
		out.Verdict = SampleUnresolved
		out.Reasons = []string{ReasonUnknownAssay}
	}
	return out
}

// dieselEpoch selects the verified rule window from the collection year.
// The 2026 guidance describes RANP nº 968/2024 as the current rule for the
// open-data period it documents, and RANP nº 50/2013 was the rule before the
// 2024 resolution existed. The exact 2024–2025 changeover is not carried by
// the verified guidance, so those years stay epoch-unknown and use the
// disagreement rule instead of an invented date.
type dieselEpochKind string

const (
	epochCurrent dieselEpochKind = "current"
	epochLegacy  dieselEpochKind = "legacy"
	epochUnknown dieselEpochKind = "unknown"
)

func dieselEpoch(observedOn string) dieselEpochKind {
	day := strings.TrimSpace(observedOn)
	if len(day) < 4 {
		return epochUnknown
	}
	switch year := day[:4]; {
	case year >= "2026":
		return epochCurrent
	case year <= "2023":
		return epochLegacy
	default:
		return epochUnknown
	}
}

// judgeDieselAspecto applies RANP nº 968/2024 (current) or RANP nº 50/2013
// (legacy) by collection epoch; when the epoch is unknown and the two rules
// disagree, the sample stays UNRESOLVED instead of guessing.
func judgeDieselAspecto(facts SampleFacts, distinct []AssayObservation, aspectResult string, out SampleJudgment) SampleJudgment {
	switch dieselEpoch(facts.ObservedOn) {
	case epochCurrent:
		verdict, reason := dieselCurrentRule(distinct, aspectResult)
		return dieselOutcome(verdict, reason, out)
	case epochLegacy:
		verdict, reason := dieselLegacyRule(distinct, facts.Fuel)
		return dieselOutcome(verdict, reason, out)
	default:
		current, currentReason := dieselCurrentRule(distinct, aspectResult)
		if current == SampleUnresolved {
			out.Verdict = SampleUnresolved
			out.Reasons = []string{currentReason}
			return out
		}
		legacy, legacyReason := dieselLegacyRule(distinct, facts.Fuel)
		if current != legacy {
			out.Verdict = SampleUnresolved
			out.Reasons = []string{ReasonDieselRuleEpoch}
			return out
		}
		if current == SampleConforming && currentReason == ReasonDieselAspectoCleared && legacyReason == ReasonAspectoUnconfirmed {
			// One verified window evaluated the complementaries and cleared
			// them; the other only lacked rows.
			currentReason = ReasonDieselAspectoCleared
		}
		return dieselOutcome(current, currentReason, out)
	}
}

func dieselOutcome(verdict SampleVerdict, reason string, out SampleJudgment) SampleJudgment {
	out.Verdict = verdict
	if reason != "" {
		out.Reasons = []string{reason}
	}
	return out
}

// dieselCurrentRule evaluates RANP nº 968/2024 art. 16 by the defect text:
// heterogeneous product fails directly; particles require total
// contamination; turbidity requires water and total contamination, where a
// first failure dispenses the second.
func dieselCurrentRule(distinct []AssayObservation, aspectResult string) (SampleVerdict, string) {
	switch classifyDieselDefect(aspectResult) {
	case defectHeterogeneous:
		return SampleNonconforming, ""
	case defectParticles:
		return outcomeOf(distinct, []string{"Contaminação total"})
	case defectTurbidity:
		return outcomeOf(distinct, []string{"Teor de Água", "Contaminação total"})
	default:
		return SampleUnresolved, ReasonDefectUnclassified
	}
}

type dieselDefect int

const (
	defectUnknown dieselDefect = iota
	defectHeterogeneous
	defectParticles
	defectTurbidity
)

// classifyDieselDefect keys on the frozen official vocabulary: heterogeneous
// text, particulate text, or a result opening with the turbidity token. Any
// other wording is unclassified rather than guessed.
func classifyDieselDefect(result string) dieselDefect {
	folded := foldASCII(strings.ToLower(strings.TrimSpace(result)))
	switch {
	case strings.Contains(folded, "heterog"):
		return defectHeterogeneous
	case strings.Contains(folded, "material particulado"):
		return defectParticles
	case strings.HasPrefix(folded, "turvo"):
		return defectTurbidity
	default:
		return defectUnknown
	}
}

// dieselLegacyRule evaluates RANP nº 50/2013: whatever the defect, at least
// one nonconforming complementary of the grade's set fails the Aspecto.
func dieselLegacyRule(distinct []AssayObservation, fuel Fuel) (SampleVerdict, string) {
	var set []string
	switch fuel {
	case FuelDieselS500:
		set = []string{"Teor de Água", "Teor de Água e Sedimentos"}
	case FuelDieselS10:
		set = []string{"Teor de Água", "Contaminação total"}
	default:
		return SampleUnresolved, ReasonDefectUnclassified
	}
	return outcomeOf(distinct, set)
}

// outcomeOf applies "any nonconforming complementary fails; every required
// complementary evaluated and conforming clears; otherwise unconfirmed".
func outcomeOf(distinct []AssayObservation, names []string) (SampleVerdict, string) {
	present, cleared := false, false
	for _, name := range names {
		for _, row := range distinct {
			if row.Ensaio != name {
				continue
			}
			present = true
			if row.Conformity == Nonconforming {
				return SampleNonconforming, ""
			}
			if row.Conformity == Conforming {
				cleared = true
			}
		}
	}
	if present && cleared {
		return SampleConforming, ReasonDieselAspectoCleared
	}
	return SampleConforming, ReasonAspectoUnconfirmed
}

// complementaryOutcome reports how the ethanol complementaries behaved.
func complementaryOutcome(distinct []AssayObservation, names []string) (cleared, confirmed, present bool) {
	for _, name := range names {
		for _, row := range distinct {
			if row.Ensaio != name {
				continue
			}
			present = true
			if row.Conformity == Nonconforming {
				confirmed = true
			}
			if row.Conformity == Conforming {
				cleared = true
			}
		}
	}
	return cleared, confirmed, present
}

func foldASCII(text string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
		"é", "e", "ê", "e", "è", "e",
		"í", "i", "î", "i",
		"ó", "o", "õ", "o", "ô", "o",
		"ú", "u", "ü", "u",
		"ç", "c",
	)
	return replacer.Replace(text)
}

// ScoreState is the frozen score availability vocabulary. UNSCORED is the
// only reachable state until calibration freezes weights/thresholds.
type ScoreState string

const ScoreUnscored ScoreState = "UNSCORED"

// FuelDimensions is the explicit dimension set for one fuel: distinct
// samples, distinct visits (station+collection day), verdict counts, the
// newest/oldest collection days and the calibration state. It carries no
// numeric score by design.
type FuelDimensions struct {
	MethodVersion    string
	Fuel             Fuel
	Samples          int64
	Visits           int64
	Conforming       int64
	Nonconforming    int64
	Unresolved       int64
	OldestObservedOn string
	NewestObservedOn string
	Score            ScoreState
	Notes            []string
}

// ProjectFuelDimensions reduces judged samples into per-fuel dimensions.
// UNRESOLVED samples are counted but excluded from quality denominators by
// consumers; no step invents a weight, threshold or score.
func ProjectFuelDimensions(samples []SampleFacts) map[Fuel]FuelDimensions {
	out := map[Fuel]FuelDimensions{}
	visits := map[Fuel]map[string]bool{}
	for _, sample := range samples {
		if strings.TrimSpace(sample.SampleKey) == "" {
			continue
		}
		dim := out[sample.Fuel]
		dim.MethodVersion = MethodologyVersion
		dim.Fuel = sample.Fuel
		dim.Score = ScoreUnscored
		dim.Notes = []string{MethodologyLimitation}
		dim.Samples++
		switch sample.Judgment.Verdict {
		case SampleConforming:
			dim.Conforming++
		case SampleNonconforming:
			dim.Nonconforming++
		case SampleUnresolved:
			dim.Unresolved++
		}
		if day := strings.TrimSpace(sample.ObservedOn); day != "" {
			day = day[:min(10, len(day))]
			if dim.OldestObservedOn == "" || day < dim.OldestObservedOn {
				dim.OldestObservedOn = day
			}
			if dim.NewestObservedOn == "" || day > dim.NewestObservedOn {
				dim.NewestObservedOn = day
			}
			visit := sample.StationID + "\x1f" + day
			if visits[sample.Fuel] == nil {
				visits[sample.Fuel] = map[string]bool{}
			}
			visits[sample.Fuel][visit] = true
		}
		out[sample.Fuel] = dim
	}
	for fuel, dim := range out {
		dim.Visits = int64(len(visits[fuel]))
		out[fuel] = dim
	}
	return out
}
