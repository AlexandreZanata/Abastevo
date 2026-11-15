package domain

// MaxExpectedSeconds bounds the expected recording time (EDR-276): the instant
// profile spans less than 3.2×10¹¹ s, so the bound only prevents overflow.
const MaxExpectedSeconds int64 = 1_000_000_000_000

// TrackEvidence holds the counted inputs of one track summary.
type TrackEvidence struct {
	ExpectedSeconds int64 // whole seconds the trip was IN_PROGRESS
	GapSeconds      int64 // whole seconds of gaps inside that time
	AcceptedFixes   int64
	Coarse          bool // any recording period was approximate
}

// Classification is the quality of one track summary with its coverage.
type Classification struct {
	quality  Quality
	coverage int64
	known    bool
}

func (c Classification) Quality() Quality { return c.quality }

// Coverage returns the permille coverage, unknown for zero expected time.
func (c Classification) Coverage() (int64, bool) { return c.coverage, c.known }

// Classify applies the track quality rule (B-BR-EMP-427, BR-GPS-026, §6.2,
// §10.4): coverage = floor((expected − gap) × 1000 / expected); INSUFFICIENT
// without expected time, with fewer than two accepted fixes or below 500;
// APPROXIMATE with any coarse period or below 900; otherwise PRECISE.
func Classify(e TrackEvidence) (Classification, error) {
	if e.ExpectedSeconds < 0 || e.GapSeconds < 0 || e.AcceptedFixes < 0 || e.ExpectedSeconds > MaxExpectedSeconds ||
		e.GapSeconds > e.ExpectedSeconds {
		return Classification{}, ErrInvalid
	}
	if e.ExpectedSeconds == 0 {
		return Classification{quality: QualityInsufficient}, nil
	}
	c := Classification{coverage: (e.ExpectedSeconds - e.GapSeconds) * 1000 / e.ExpectedSeconds, known: true}
	switch {
	case e.AcceptedFixes < 2 || c.coverage < 500:
		c.quality = QualityInsufficient
	case e.Coarse || c.coverage < 900:
		c.quality = QualityApproximate
	default:
		c.quality = QualityPrecise
	}
	return c, nil
}
