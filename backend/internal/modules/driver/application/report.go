package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
)

const reportLimit = domain.MaxItems
const maxReportDays = 366

var ErrStale = errors.New("driver: stale report revision")
var ErrReportTooLarge = errors.New("driver: report source exceeds bounded limit")

type ReportRequest struct {
	VehicleID        string `json:"vehicle_id"`
	Timezone         string `json:"timezone"`
	Period           string `json:"period"`
	Anchor           string `json:"anchor,omitempty"`
	From             string `json:"from,omitempty"`
	To               string `json:"to,omitempty"`
	Platform         string `json:"platform,omitempty"`
	Basis            string `json:"basis"`
	ExpectedRevision string `json:"expected_revision,omitempty"`
}

type ReportSnapshotKey struct {
	VehicleID, Period, From, To, Timezone, Platform, Basis string
}

type Report struct {
	Revision               string            `json:"revision"`
	SnapshotVersion        int64             `json:"snapshot_version"`
	VehicleID              string            `json:"vehicle_id"`
	Timezone               string            `json:"timezone"`
	Period                 string            `json:"period"`
	From                   string            `json:"from"`
	To                     string            `json:"to"`
	Platform               string            `json:"platform,omitempty"`
	Basis                  string            `json:"basis"`
	Currency               string            `json:"currency"`
	EarnedCentavos         *int64            `json:"earned_centavos"`
	AdjustmentsCentavos    *int64            `json:"adjustments_centavos"`
	CostsCentavos          *int64            `json:"costs_centavos"`
	ModeledCostsCentavos   *int64            `json:"modeled_costs_centavos"`
	RecordedResultCentavos *int64            `json:"recorded_result_centavos"`
	ModeledResultCentavos  *int64            `json:"modeled_result_centavos"`
	DistanceMetres         *int64            `json:"distance_metres"`
	EarningsPerKm          string            `json:"earnings_per_km"`
	RecordedPerKm          string            `json:"recorded_result_per_km"`
	RecordedPerHour        string            `json:"recorded_result_per_hour"`
	ModeledPerKm           string            `json:"modeled_result_per_km"`
	ModeledPerHour         string            `json:"modeled_result_per_hour"`
	RatiosUnknown          string            `json:"ratios_unknown"`
	Coverage               map[string]string `json:"coverage"`
	Methods                map[string]string `json:"methods"`
	GeneratedAt            time.Time         `json:"generated_at"`
}

type StoredReportSnapshot struct {
	Key             ReportSnapshotKey
	SourceRevision  string
	SnapshotVersion int64
	GeneratedAt     time.Time
	Payload         []byte
}

type ReportSnapshotStore interface {
	SaveReportSnapshot(context.Context, string, ReportSnapshotKey, string, []byte) (int64, time.Time, error)
	GetReportSnapshot(context.Context, string, ReportSnapshotKey) (StoredReportSnapshot, error)
	DeleteExpiredReportSnapshots(context.Context, time.Time, int) (int64, error)
}

type ReportService struct {
	Store     DriverStore
	Snapshots ReportSnapshotStore
	Paid      func(context.Context, string) error
	Now       func() time.Time
}

type reportSource struct {
	fromDay, toDay, from, to string
	fromTime, toTime         time.Time
	location                 *time.Location
	earnings                 []StoredEarning
	adjustments              map[string][]StoredAdjustment
	earningsCoverage         string
	costProfile              *domain.VehicleCostProfile
	costsCoverage            string
	actualCosts              []domain.Cost
	modeledCosts             []domain.Cost
	costsComplete            bool
	estimatesKnown           bool
	actualTotal              *int64
	modeledTotal             *int64
	distance                 []domain.DistanceRecord
	revision                 string
}

func (s *ReportService) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (r ReportRequest) validate() (ReportSnapshotKey, *time.Location, string, string, time.Time, time.Time, error) {
	if !domain.ValidID(r.VehicleID) || r.Timezone == "" || r.Timezone == "Local" || r.Basis == "" {
		return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, domain.ErrInvalid
	}
	switch r.Basis {
	case "ODOMETER", "GPS_ESTIMATE", "PAID_REPORTED", "MANUAL_TRIP":
	default:
		return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, domain.ErrInvalid
	}
	if r.Platform != "" && r.Platform != "UBER" && r.Platform != "99" && r.Platform != "INDRIVE" && r.Platform != "OTHER" {
		return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, domain.ErrInvalid
	}
	loc, e := time.LoadLocation(r.Timezone)
	if e != nil {
		return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, domain.ErrInvalid
	}
	day := func(raw string) (time.Time, error) {
		d, e := time.ParseInLocation("2006-01-02", raw, loc)
		if e != nil || d.Format("2006-01-02") != raw {
			return time.Time{}, domain.ErrInvalid
		}
		return d, nil
	}
	var from, to time.Time
	switch r.Period {
	case "DAY", "WEEK", "MONTH":
		anchor := r.Anchor
		if anchor == "" {
			now := s2now(loc)
			anchor = now.Format("2006-01-02")
		}
		d, e := day(anchor)
		if e != nil {
			return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, e
		}
		switch r.Period {
		case "DAY":
			from, to = d, d.AddDate(0, 0, 1)
		case "WEEK":
			weekday := int(d.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			from = d.AddDate(0, 0, -(weekday - 1))
			to = from.AddDate(0, 0, 7)
		case "MONTH":
			from = time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, loc)
			to = from.AddDate(0, 1, 0)
		}
	case "CUSTOM":
		if r.From == "" || r.To == "" {
			return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, domain.ErrInvalid
		}
		var e error
		if from, e = day(r.From); e != nil {
			return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, e
		}
		if to, e = day(r.To); e != nil {
			return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, e
		}
		if !to.After(from) || to.Sub(from) > maxReportDays*24*time.Hour {
			return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, domain.ErrInvalid
		}
	default:
		return ReportSnapshotKey{}, nil, "", "", time.Time{}, time.Time{}, domain.ErrInvalid
	}
	fromDay, toDay := from.Format("2006-01-02"), to.Format("2006-01-02")
	key := ReportSnapshotKey{VehicleID: r.VehicleID, Period: r.Period, From: fromDay, To: toDay, Timezone: r.Timezone, Platform: r.Platform, Basis: r.Basis}
	return key, loc, fromDay, toDay, from.UTC(), to.UTC(), nil
}

// s2now is a seam for the default anchor; tests use an explicit anchor.
func s2now(loc *time.Location) time.Time { return time.Now().In(loc) }

func (s *ReportService) Build(ctx context.Context, owner string, req ReportRequest) (Report, error) {
	if s.Paid == nil {
		return Report{}, ErrDenied
	}
	if e := s.Paid(ctx, owner); e != nil {
		return Report{}, e
	}
	key, loc, fromDay, toDay, from, to, e := req.validate()
	if e != nil {
		return Report{}, e
	}
	source, e := s.load(ctx, owner, req, key, loc, fromDay, toDay, from, to)
	if e != nil {
		return Report{}, e
	}
	if req.ExpectedRevision != "" && req.ExpectedRevision != source.revision {
		return Report{}, ErrStale
	}
	report := buildReport(owner, req, key, source, s.now())
	if s.Snapshots == nil || s.Store == nil {
		return Report{}, ErrUnavailable
	}
	payload, e := jsonMarshal(report)
	if e != nil {
		return Report{}, e
	}
	version, generated, e := s.Snapshots.SaveReportSnapshot(ctx, owner, key, source.revision, payload)
	if e != nil {
		return Report{}, e
	}
	report.SnapshotVersion = version
	report.GeneratedAt = generated.UTC()
	return report, nil
}

func (s *ReportService) Get(ctx context.Context, owner string, req ReportRequest) (Report, error) {
	key, loc, fromDay, toDay, from, to, e := req.validate()
	if e != nil {
		return Report{}, e
	}
	if s.Snapshots == nil || s.Store == nil {
		return Report{}, ErrUnavailable
	}
	stored, e := s.Snapshots.GetReportSnapshot(ctx, owner, key)
	if e != nil {
		return Report{}, e
	}
	source, e := s.load(ctx, owner, req, key, loc, fromDay, toDay, from, to)
	if e != nil {
		return Report{}, e
	}
	if stored.SourceRevision != source.revision {
		return Report{}, ErrStale
	}
	report, e := reportFromPayload(stored.Payload)
	if e != nil {
		return Report{}, e
	}
	if req.ExpectedRevision != "" && req.ExpectedRevision != report.Revision {
		return Report{}, ErrStale
	}
	report.SnapshotVersion = stored.SnapshotVersion
	report.GeneratedAt = stored.GeneratedAt.UTC()
	return report, nil
}

func (s *ReportService) load(ctx context.Context, owner string, req ReportRequest, key ReportSnapshotKey, loc *time.Location, fromDay, toDay string, from, to time.Time) (reportSource, error) {
	out := reportSource{fromDay: fromDay, toDay: toDay, from: fromDay, to: toDay, fromTime: from, toTime: to, location: loc, adjustments: map[string][]StoredAdjustment{}, earningsCoverage: "complete", costsCoverage: "complete", costsComplete: true, estimatesKnown: true}
	if s.Store == nil {
		return out, ErrUnavailable
	}
	tokens := []string{"r|" + owner + "|" + key.VehicleID + "|" + key.Period + "|" + fromDay + "|" + toDay + "|" + key.Timezone + "|" + key.Platform + "|" + key.Basis}

	after := ""
	scanned := 0
	for {
		page, e := s.Store.List(ctx, owner, after, reportLimit)
		if e != nil {
			return out, e
		}
		scanned += len(page.Records)
		if scanned > reportLimit && page.NextID != "" {
			return out, ErrReportTooLarge
		}
		for _, stored := range page.Records {
			r := stored.Record
			if r.VehicleID != req.VehicleID || (req.Platform != "" && r.Platform != req.Platform) {
				continue
			}
			if r.From.Before(from) || r.To.After(to) {
				if r.From.Before(to) && r.To.After(from) {
					out.earningsCoverage = "partial-window"
				}
				continue
			}
			out.earnings = append(out.earnings, stored)
			earned := mustEarned(r)
			tokens = append(tokens, "e|"+r.ID+"|"+itoa(stored.Revision)+"|"+itoa(earned))
			if stored.AdjustmentCount > 0 {
				adjustments, next, e := s.loadAdjustments(ctx, owner, r.ID)
				if e != nil {
					return out, e
				}
				if next != "" {
					return out, ErrReportTooLarge
				}
				out.adjustments[r.ID] = adjustments
				for _, a := range adjustments {
					tokens = append(tokens, "a|"+a.Adjustment.ID+"|"+itoa(a.Adjustment.TargetRevision)+"|"+itoa(a.Adjustment.Delta))
					if stored.NeedsReconciliation || a.Adjustment.TargetRevision != stored.Revision {
						out.earningsCoverage = "adjustment-revision-obsolete"
					}
				}
			} else if stored.NeedsReconciliation {
				out.earningsCoverage = "adjustment-revision-obsolete"
			}
		}
		if page.NextID == "" {
			break
		}
		if scanned >= reportLimit {
			return out, ErrReportTooLarge
		}
		after = page.NextID
	}
	if len(out.earnings) == 0 {
		out.earningsCoverage = "no-records"
	}

	profiles, e := s.Store.ListCostProfiles(ctx, owner, req.VehicleID, 0, reportLimit)
	if e != nil {
		return out, e
	}
	if len(profiles) >= reportLimit {
		return out, ErrReportTooLarge
	}
	for i := range profiles {
		p := &profiles[i]
		if p.From <= fromDay && (p.Until == "" || p.Until > fromDay) {
			out.costProfile = p
		}
	}
	if out.costProfile == nil {
		out.costsCoverage = "profile-missing"
		out.costsComplete = false
		out.estimatesKnown = false
		tokens = append(tokens, "p|missing")
	} else {
		cp := out.costProfile
		tokens = append(tokens, "p|"+itoa(cp.Version))
		if cp.From > fromDay || (cp.Until != "" && cp.Until < toDay) {
			out.costsCoverage = "profile-window-partial"
			out.costsComplete = false
		}
		actual := make([]int64, 0)
		modeled := make([]int64, 0)
		for _, c := range cp.Costs {
			if c.From >= toDay || (c.Until != "" && c.Until <= fromDay) {
				continue
			}
			if c.From < fromDay || c.Until == "" || c.Until > toDay {
				out.costsCoverage = "cost-item-window-partial"
				out.costsComplete = false
				if c.Provenance == "MODELED" {
					out.estimatesKnown = false
				}
				continue
			}
			if c.Amount == nil {
				out.costsCoverage = "cost-amount-unknown"
				out.costsComplete = false
				if c.Provenance == "MODELED" {
					out.estimatesKnown = false
				}
				continue
			}
			cost := domain.Cost{ID: c.ID, Amount: *c.Amount, Window: domain.Window{Owner: owner, Vehicle: req.VehicleID, Currency: "BRL", FromMillis: from.UnixMilli(), ToMillis: to.UnixMilli()}}
			if c.Provenance == "MODELED" {
				modeled = append(modeled, *c.Amount)
				out.modeledCosts = append(out.modeledCosts, cost)
			} else {
				actual = append(actual, *c.Amount)
				out.actualCosts = append(out.actualCosts, cost)
			}
			tokens = append(tokens, "c|"+c.ID+"|"+c.Kind+"|"+c.Provenance+"|"+itoa(*c.Amount))
		}
		if out.costsComplete {
			out.actualTotal, _ = sumAmounts(actual)
		}
		if out.estimatesKnown {
			out.modeledTotal, _ = sumAmounts(modeled)
			if out.modeledCosts == nil {
				out.modeledCosts = []domain.Cost{}
			}
		}
	}

	distances, e := s.Store.ListDistances(ctx, owner, req.VehicleID, from, to, reportLimit)
	if e != nil {
		return out, e
	}
	if len(distances) >= reportLimit {
		return out, ErrReportTooLarge
	}
	out.distance = distances
	for _, d := range distances {
		hash, e := d.ContentHash()
		if e != nil {
			return out, e
		}
		tokens = append(tokens, "d|"+d.ID+"|"+hash)
	}
	sort.Strings(tokens)
	h := sha256.New()
	for _, token := range tokens {
		h.Write([]byte(token))
		h.Write([]byte{0})
	}
	out.revision = hex.EncodeToString(h.Sum(nil))
	return out, nil
}

func (s *ReportService) loadAdjustments(ctx context.Context, owner, id string) ([]StoredAdjustment, string, error) {
	page, e := s.Store.ListAdjustments(ctx, owner, id, "", reportLimit)
	if e != nil {
		return nil, "", e
	}
	return page.Adjustments, page.NextID, nil
}

func mustEarned(r domain.EarningRecord) int64 {
	n, e := r.Money.Earned()
	if e != nil {
		return 0
	}
	return n
}

func addAmounts(a, b int64) (int64, error) {
	n := new(big.Int).Add(big.NewInt(a), big.NewInt(b))
	if !n.IsInt64() || n.Cmp(big.NewInt(domain.MaxAmount)) > 0 || n.Cmp(big.NewInt(-domain.MaxAmount)) < 0 {
		return 0, domain.ErrInvalid
	}
	return n.Int64(), nil
}

func sumAmounts(values []int64) (*int64, error) {
	n := new(big.Int)
	for _, value := range values {
		if value < -domain.MaxAmount || value > domain.MaxAmount {
			return nil, domain.ErrInvalid
		}
		n.Add(n, big.NewInt(value))
	}
	if !n.IsInt64() || n.Cmp(big.NewInt(domain.MaxAmount)) > 0 || n.Cmp(big.NewInt(-domain.MaxAmount)) < 0 {
		return nil, domain.ErrInvalid
	}
	v := n.Int64()
	return &v, nil
}

func buildReport(owner string, req ReportRequest, key ReportSnapshotKey, source reportSource, now time.Time) Report {
	out := Report{
		Revision: source.revision, VehicleID: key.VehicleID, Timezone: key.Timezone, Period: key.Period,
		From: key.From, To: key.To, Platform: key.Platform, Basis: key.Basis, Currency: "BRL",
		Coverage: map[string]string{"earnings": source.earningsCoverage, "costs": source.costsCoverage, "distance": "unknown", "time": "time-not-projected", "result": "unknown"},
		Methods:  map[string]string{"money": "exact-int64-v1", "costs": "linked-profile-exact-window-v1", "distance": "select-one-basis-v1", "ratios": "exact-rational-v1"}, GeneratedAt: now,
	}
	earnedValues := make([]int64, 0, len(source.earnings))
	adjustmentValues := make([]int64, 0)
	adjustmentsKnown := true
	for _, stored := range source.earnings {
		earned, e := stored.Record.Money.Earned()
		if e != nil {
			adjustmentsKnown = false
			continue
		}
		earnedValues = append(earnedValues, earned)
		for _, a := range source.adjustments[stored.Record.ID] {
			if a.Adjustment.TargetRevision != stored.Revision {
				adjustmentsKnown = false
				continue
			}
			adjustmentValues = append(adjustmentValues, a.Adjustment.Delta)
		}
	}
	if source.earningsCoverage == "adjustment-revision-obsolete" {
		adjustmentsKnown = false
	}
	totalEarned, e := sumAmounts(earnedValues)
	if e != nil {
		adjustmentsKnown = false
		zero := int64(0)
		totalEarned = &zero
	}
	out.EarnedCentavos = totalEarned
	earnedWithAdjustments := *totalEarned
	if adjustmentsKnown {
		adjTotal, e := sumAmounts(adjustmentValues)
		if e != nil {
			adjustmentsKnown = false
		} else {
			earnedWithAdjustments, e = addAmounts(*totalEarned, *adjTotal)
			if e != nil {
				adjustmentsKnown = false
			} else {
				out.AdjustmentsCentavos = adjTotal
			}
		}
	}
	out.CostsCentavos = source.actualTotal
	out.ModeledCostsCentavos = source.modeledTotal

	rw := domain.Window{Owner: owner, Vehicle: key.VehicleID, Currency: "BRL", FromMillis: source.fromTime.UnixMilli(), ToMillis: source.toTime.UnixMilli()}
	observations := make([]domain.DistanceObservation, 0, len(source.distance))
	for _, d := range source.distance {
		if d.From.UnixMilli() == rw.FromMillis && d.To.UnixMilli() == rw.ToMillis {
			observations = append(observations, domain.DistanceObservation{ID: d.ID, Segment: d.Segment, Basis: d.Basis, Window: rw, Metres: d.Metres, Complete: d.Complete, HasGap: d.HasGap})
		}
	}
	selected, e := domain.SelectDistance(rw, req.Basis, observations)
	if e == nil && selected.Metres != nil {
		out.DistanceMetres = selected.Metres
		out.Coverage["distance"] = "complete"
	} else if e != nil {
		out.Coverage["distance"] = "invalid"
	} else {
		out.Coverage["distance"] = selected.Unknown
	}
	var estimates []domain.Cost
	if source.estimatesKnown {
		estimates = source.modeledCosts
		if estimates == nil {
			estimates = []domain.Cost{}
		}
	}
	complete := source.costsComplete && adjustmentsKnown && source.earningsCoverage != "partial-window"
	result, e := domain.Result(earnedWithAdjustments, rw, rw, source.actualCosts, complete, estimates, out.DistanceMetres, nil)
	if e != nil {
		out.Coverage["result"] = "invalid"
		out.RatiosUnknown = "invalid"
		return out
	}
	out.RecordedResultCentavos = result.Recorded
	out.ModeledResultCentavos = result.Modeled
	out.RecordedPerKm = result.RecordedRatios.PerKm
	out.RecordedPerHour = result.RecordedRatios.PerHour
	out.ModeledPerKm = result.ModeledRatios.PerKm
	out.ModeledPerHour = result.ModeledRatios.PerHour
	out.RatiosUnknown = strings.Trim(result.RecordedRatios.Unknown+","+result.ModeledRatios.Unknown+","+result.Unknown, ",")
	if result.Unknown == "" {
		out.Coverage["result"] = "complete"
	} else {
		out.Coverage["result"] = result.Unknown
	}
	earningsRatios, e := domain.ExactRatios(earnedWithAdjustments, out.DistanceMetres, nil)
	if e == nil {
		out.EarningsPerKm = earningsRatios.PerKm
	}
	return out
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func reportFromPayload(payload []byte) (Report, error) {
	var out Report
	e := json.Unmarshal(payload, &out)
	return out, e
}
