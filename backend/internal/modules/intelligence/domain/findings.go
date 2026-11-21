package domain

import (
	"errors"
	"math/big"
	"strings"
)

const FindingsMethodVersion = "findings-v1"

var (
	ErrFindingClosed        = errors.New("intelligence: finding is in terminal state")
	ErrFindingConflict      = errors.New("intelligence: finding revision conflict")
	ErrFindingSelfApproval  = errors.New("intelligence: author cannot decide own finding")
	ErrJustificationMissing = errors.New("intelligence: justification required")
	ErrThresholdOutOfRange  = errors.New("intelligence: threshold out of allowed range")
)

type FindingType string

const (
	FindingDuplicateFueling    FindingType = "FT-001"
	FindingIncoherentReading   FindingType = "FT-002"
	FindingTankOverCapacity    FindingType = "FT-003"
	FindingConsumptionOutlier  FindingType = "FT-004"
	FindingDocumentDivergent   FindingType = "FT-005"
	FindingPriceAboveReference FindingType = "FT-006"
)

type FindingSeverity string

const (
	SeverityLow    FindingSeverity = "LOW"
	SeverityMedium FindingSeverity = "MEDIUM"
	SeverityHigh   FindingSeverity = "HIGH"
)

type FindingState string

const (
	FindingOpen          FindingState = "OPEN"
	FindingInReview      FindingState = "IN_REVIEW"
	FindingConfirmed     FindingState = "CONFIRMED"
	FindingExplained     FindingState = "EXPLAINED"
	FindingFalsePositive FindingState = "FALSE_POSITIVE"
	FindingSuperseded    FindingState = "SUPERSEDED"
)

type FindingSignal struct {
	Raised   bool
	Type     FindingType
	Severity FindingSeverity
	Title    string
	NonProof string
}

type FindingSettings struct {
	DuplicateWindowMinutes  int
	MaxSpeedKmh             int
	TankTolerancePercent    int
	OutlierThresholdPercent int
	DocQtyTolerancePercent  int
	DocDateToleranceDays    int
	PriceThresholdPercent   int
	PriceMinDiffCents       int64
}

func DefaultFindingSettings() FindingSettings {
	return FindingSettings{
		DuplicateWindowMinutes:  30,
		MaxSpeedKmh:             150,
		TankTolerancePercent:    5,
		OutlierThresholdPercent: 20,
		DocQtyTolerancePercent:  1,
		DocDateToleranceDays:    1,
		PriceThresholdPercent:   5,
		PriceMinDiffCents:       1000,
	}
}

func (s FindingSettings) Validate() error {
	if s.DuplicateWindowMinutes < 5 || s.DuplicateWindowMinutes > 1440 ||
		s.MaxSpeedKmh < 80 || s.MaxSpeedKmh > 200 ||
		s.TankTolerancePercent < 0 || s.TankTolerancePercent > 20 ||
		s.OutlierThresholdPercent < 10 || s.OutlierThresholdPercent > 50 ||
		s.DocQtyTolerancePercent < 0 || s.DocQtyTolerancePercent > 5 ||
		s.DocDateToleranceDays < 0 || s.DocDateToleranceDays > 7 ||
		s.PriceThresholdPercent < 1 || s.PriceThresholdPercent > 30 ||
		s.PriceMinDiffCents < 0 || s.PriceMinDiffCents > 50000 {
		return ErrThresholdOutOfRange
	}
	return nil
}

func EvaluateFT001Duplicate(deltaMinutes int, windowMinutes int, sameKeyOrHash bool) FindingSignal {
	if !sameKeyOrHash && deltaMinutes > windowMinutes {
		return FindingSignal{}
	}
	sev := SeverityMedium
	if sameKeyOrHash {
		sev = SeverityHigh
	}
	return FindingSignal{
		Raised:   true,
		Type:     FindingDuplicateFueling,
		Severity: sev,
		Title:    "Possível abastecimento duplicado para revisão",
		NonProof: "Não comprova fraude ou pagamento duplicado; dois abastecimentos reais ou rateio de um cupom podem coincidir.",
	}
}

func EvaluateFT002IncoherentMeter(decreased bool, deltaM, deltaS int64, maxSpeedKmh int, hourMeterExceeded bool) FindingSignal {
	speedExceeded := false
	if deltaS > 0 && deltaM > 0 {
		// deltaM * 3600 > maxSpeedKmh * 1000 * deltaS
		lhs := new(big.Int).Mul(big.NewInt(deltaM), big.NewInt(3600))
		rhs := new(big.Int).Mul(big.NewInt(int64(maxSpeedKmh)*1000), big.NewInt(deltaS))
		speedExceeded = lhs.Cmp(rhs) > 0
	}
	if !decreased && !speedExceeded && !hourMeterExceeded {
		return FindingSignal{}
	}
	return FindingSignal{
		Raised:   true,
		Type:     FindingIncoherentReading,
		Severity: SeverityMedium,
		Title:    "Leitura inconsistente com a sequência do medidor",
		NonProof: "Não comprova adulteração; erros de digitação, foto ilegível ou troca de medidor podem ocorrer.",
	}
}

func EvaluateFT003TankCapacity(qtyMilli, capacityMilli int64, tolerancePct int, incompatibleProduct bool) FindingSignal {
	if incompatibleProduct {
		return FindingSignal{
			Raised:   true,
			Type:     FindingTankOverCapacity,
			Severity: SeverityHigh,
			Title:    "Produto incompatível com o tanque cadastrado",
			NonProof: "Não comprova desvio; o cadastro do tanque pode estar desatualizado.",
		}
	}
	if capacityMilli <= 0 {
		return FindingSignal{}
	}
	limit := new(big.Int).Quo(new(big.Int).Mul(big.NewInt(capacityMilli), big.NewInt(int64(100+tolerancePct))), big.NewInt(100))
	if big.NewInt(qtyMilli).Cmp(limit) <= 0 {
		return FindingSignal{}
	}
	sev := SeverityMedium
	if new(big.Int).Mul(big.NewInt(qtyMilli), big.NewInt(100)).Cmp(new(big.Int).Mul(big.NewInt(capacityMilli), big.NewInt(120))) > 0 {
		sev = SeverityHigh
	}
	return FindingSignal{
		Raised:   true,
		Type:     FindingTankOverCapacity,
		Severity: sev,
		Title:    "Volume acima da capacidade cadastrada do tanque",
		NonProof: "Não comprova desvio de combustível; tanque auxiliar ou erro de capacidade podem explicar.",
	}
}

func EvaluateFT004ConsumptionOutlier(baseM, baseMl, testM, testMl int64, thresholdPct int) FindingSignal {
	if baseM <= 0 || baseMl <= 0 || testM <= 0 || testMl <= 0 {
		return FindingSignal{}
	}
	// ratio = (testM * baseMl) / (testMl * baseM); diffPct = |ratio - 1| * 100
	num := new(big.Int).Mul(big.NewInt(testM), big.NewInt(baseMl))
	den := new(big.Int).Mul(big.NewInt(testMl), big.NewInt(baseM))
	diff := new(big.Int).Abs(new(big.Int).Sub(num, den))
	// Check diff * 100 > den * thresholdPct
	if new(big.Int).Mul(diff, big.NewInt(100)).Cmp(new(big.Int).Mul(den, big.NewInt(int64(thresholdPct)))) <= 0 {
		return FindingSignal{}
	}
	sev := SeverityMedium
	if new(big.Int).Mul(diff, big.NewInt(100)).Cmp(new(big.Int).Mul(den, big.NewInt(int64(thresholdPct*2)))) > 0 {
		sev = SeverityHigh
	}
	return FindingSignal{
		Raised:   true,
		Type:     FindingConsumptionOutlier,
		Severity: sev,
		Title:    "Consumo fora da faixa histórica do ativo",
		NonProof: "Não comprova irregularidade; carga, trajeto, clima ou abastecimento omitido alteram o consumo.",
	}
}

func EvaluateFT005DocDivergence(recCents, docCents int64, issuerMismatch bool) FindingSignal {
	diff := recCents - docCents
	if diff < 0 {
		diff = -diff
	}
	if !issuerMismatch && diff <= 1 {
		return FindingSignal{}
	}
	sev := SeverityMedium
	if issuerMismatch || (recCents > 0 && diff*100 > recCents*5) {
		sev = SeverityHigh
	}
	return FindingSignal{
		Raised:   true,
		Type:     FindingDocumentDivergent,
		Severity: sev,
		Title:    "Documento divergente do registro operacional",
		NonProof: "Não avalia autenticidade fiscal; pode ser anexo trocado ou erro de leitura.",
	}
}

func EvaluateFT006PriceAboveRef(unitMilli, refMilli, diffCents int64, thresholdPct int, minDiffCents int64) FindingSignal {
	if unitMilli <= 0 || refMilli <= 0 || diffCents < minDiffCents {
		return FindingSignal{}
	}
	lhs := new(big.Int).Mul(big.NewInt(unitMilli), big.NewInt(100))
	rhs := new(big.Int).Mul(big.NewInt(refMilli), big.NewInt(int64(100+thresholdPct)))
	if lhs.Cmp(rhs) <= 0 {
		return FindingSignal{}
	}
	sev := SeverityLow
	rhs20 := new(big.Int).Mul(big.NewInt(refMilli), big.NewInt(120))
	rhs10 := new(big.Int).Mul(big.NewInt(refMilli), big.NewInt(110))
	if lhs.Cmp(rhs20) >= 0 {
		sev = SeverityHigh
	} else if lhs.Cmp(rhs10) >= 0 {
		sev = SeverityMedium
	}
	return FindingSignal{
		Raised:   true,
		Type:     FindingPriceAboveReference,
		Severity: sev,
		Title:    "Preço registrado acima da referência comparável",
		NonProof: "Não comprova perda ou sobrepreço; a referência vem de outros postos e instantes.",
	}
}

func ValidateFindingDecision(currentState FindingState, authorID, actorID, reason string) error {
	if currentState == FindingConfirmed || currentState == FindingExplained || currentState == FindingFalsePositive || currentState == FindingSuperseded {
		return ErrFindingClosed
	}
	if authorID != "" && authorID == actorID {
		return ErrFindingSelfApproval
	}
	if strings.TrimSpace(reason) == "" {
		return ErrJustificationMissing
	}
	return nil
}
