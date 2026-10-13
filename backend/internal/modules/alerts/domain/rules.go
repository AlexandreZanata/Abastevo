// Package domain holds the frozen advanced-alert rule model (PRM07-T01).
// Bounded predicates with explicit AND/OR, frozen targets, source-aware
// baselines and deterministic reason codes. No arbitrary expressions and
// no exact-clock delivery promise.
package domain

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

type Source string

const (
	Official  Source = "official"
	Community Source = "community"
)

type PredicateKind string

const (
	Threshold PredicateKind = "threshold"
	Change    PredicateKind = "change"
)

type Predicate struct {
	Kind           PredicateKind        `json:"kind"`
	Fuel           kernel.Product       `json:"fuel"`
	Unit           kernel.Unit          `json:"unit"`
	Condition      kernel.ConditionKind `json:"condition"`
	Source         Source               `json:"source"`
	ThresholdMilli int64                `json:"threshold_milli"`
	DropMilli      int64                `json:"drop_milli,omitempty"`
	DropPercentBP  int64                `json:"drop_percent_bp,omitempty"`
	BaselineHours  int64                `json:"baseline_hours,omitempty"`
}

type Target struct {
	StationIDs []string `json:"station_ids,omitempty"`
	City       string   `json:"city,omitempty"`
}

type Schedule struct {
	Days          []int  `json:"days"`
	WindowStart   string `json:"window_start"`
	WindowEnd     string `json:"window_end"`
	Timezone      string `json:"timezone"`
	QuietStart    string `json:"quiet_start"`
	QuietEnd      string `json:"quiet_end"`
	CooldownHours int64  `json:"cooldown_hours"`
}

type Rule struct {
	Predicates []Predicate `json:"predicates"`
	Operator   string      `json:"operator"`
	Target     Target      `json:"target"`
	Schedule   Schedule    `json:"schedule"`
}

var (
	ErrInvalidPredicate = errors.New("alerts: invalid predicate")
	ErrInvalidTarget    = errors.New("alerts: invalid target")
	ErrInvalidSchedule  = errors.New("alerts: invalid schedule")
	ErrInvalidRule      = errors.New("alerts: invalid rule")
)

func validUUID(s string) bool {
	if s == "00000000-0000-0000-0000-000000000000" || len(s) != 36 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil
}

func NormalizePredicate(p Predicate) (Predicate, error) {
	switch p.Kind {
	case Threshold, Change:
	default:
		return Predicate{}, ErrInvalidPredicate
	}
	switch p.Fuel {
	case kernel.Ethanol, kernel.GasolineRegular, kernel.GasolineAdditived, kernel.GasolinePremiumGrade, kernel.DieselS500, kernel.DieselS10, kernel.CNG:
	default:
		return Predicate{}, ErrInvalidPredicate
	}
	wantUnit, err := p.Fuel.Unit()
	if err != nil {
		return Predicate{}, ErrInvalidPredicate
	}
	if p.Unit != wantUnit {
		return Predicate{}, ErrInvalidPredicate
	}
	switch p.Condition {
	case kernel.Standard, kernel.Cash, kernel.Debit, kernel.Credit, kernel.App, kernel.Loyalty, kernel.Other:
	default:
		return Predicate{}, ErrInvalidPredicate
	}
	switch p.Source {
	case Official, Community:
	default:
		return Predicate{}, ErrInvalidPredicate
	}
	if p.Kind == Threshold {
		if p.ThresholdMilli < kernel.MinMilliBRL || p.ThresholdMilli > kernel.MaxMilliBRL {
			return Predicate{}, ErrInvalidPredicate
		}
		if p.DropMilli != 0 || p.DropPercentBP != 0 || p.BaselineHours != 0 {
			return Predicate{}, ErrInvalidPredicate
		}
	} else {
		if p.ThresholdMilli != 0 {
			return Predicate{}, ErrInvalidPredicate
		}
		if p.DropMilli < 0 || p.DropMilli > kernel.MaxMilliBRL || p.DropPercentBP < 0 || p.DropPercentBP > 10000 {
			return Predicate{}, ErrInvalidPredicate
		}
		if p.DropMilli == 0 && p.DropPercentBP == 0 {
			return Predicate{}, ErrInvalidPredicate
		}
		if p.BaselineHours < 1 || p.BaselineHours > 168 {
			return Predicate{}, ErrInvalidPredicate
		}
	}
	return p, nil
}

func NormalizeTarget(t Target) (Target, error) {
	hasStations := len(t.StationIDs) > 0
	hasCity := strings.TrimSpace(t.City) != ""
	if hasStations == hasCity || (!hasStations && !hasCity) {
		return Target{}, ErrInvalidTarget
	}
	if hasStations {
		if len(t.StationIDs) > 10 {
			return Target{}, ErrInvalidTarget
		}
		seen := map[string]bool{}
		for _, id := range t.StationIDs {
			if !validUUID(id) || seen[id] {
				return Target{}, ErrInvalidTarget
			}
			seen[id] = true
		}
	}
	if hasCity && (len(t.City) == 0 || len(t.City) > 80) {
		return Target{}, ErrInvalidTarget
	}
	return t, nil
}

func parseMinutes(s string) (int, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, ErrInvalidSchedule
	}
	hour, minute := 0, 0
	for _, r := range s[:2] {
		if r < '0' || r > '9' {
			return 0, ErrInvalidSchedule
		}
		hour = hour*10 + int(r-'0')
	}
	for _, r := range s[3:] {
		if r < '0' || r > '9' {
			return 0, ErrInvalidSchedule
		}
		minute = minute*10 + int(r-'0')
	}
	if hour > 23 || minute > 59 {
		return 0, ErrInvalidSchedule
	}
	return hour*60 + minute, nil
}

func NormalizeSchedule(s Schedule) (Schedule, error) {
	if len(s.Days) < 1 || len(s.Days) > 7 {
		return Schedule{}, ErrInvalidSchedule
	}
	seen := map[int]bool{}
	for _, d := range s.Days {
		if d < 0 || d > 6 || seen[d] {
			return Schedule{}, ErrInvalidSchedule
		}
		seen[d] = true
	}
	for _, window := range []string{s.WindowStart, s.WindowEnd, s.QuietStart, s.QuietEnd} {
		if _, err := parseMinutes(window); err != nil {
			return Schedule{}, err
		}
	}
	if len(s.Timezone) == 0 || len(s.Timezone) > 64 {
		return Schedule{}, ErrInvalidSchedule
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return Schedule{}, ErrInvalidSchedule
	}
	if s.CooldownHours < 0 || s.CooldownHours > 168 {
		return Schedule{}, ErrInvalidSchedule
	}
	return s, nil
}

func NormalizeRule(r Rule) (Rule, error) {
	if len(r.Predicates) < 1 || len(r.Predicates) > 3 {
		return Rule{}, ErrInvalidRule
	}
	if r.Operator != "AND" && r.Operator != "OR" {
		return Rule{}, ErrInvalidRule
	}
	for i, p := range r.Predicates {
		normalized, err := NormalizePredicate(p)
		if err != nil {
			return Rule{}, err
		}
		r.Predicates[i] = normalized
	}
	target, err := NormalizeTarget(r.Target)
	if err != nil {
		return Rule{}, err
	}
	r.Target = target
	schedule, err := NormalizeSchedule(r.Schedule)
	if err != nil {
		return Rule{}, err
	}
	r.Schedule = schedule
	return r, nil
}

type Price struct {
	Source    Source
	Fuel      kernel.Product
	Unit      kernel.Unit
	Condition kernel.ConditionKind
	Milli     int64
	At        time.Time
}

func inWindow(minutes, start, end int) bool {
	if start == end {
		return true
	}
	if start < end {
		return minutes >= start && minutes < end
	}
	return minutes >= start || minutes < end
}

// WindowContains reports whether local minutes fall inside a HH:MM window.
// Equal bounds select the full day; inverted bounds wrap past midnight.
func WindowContains(minutes int, start, end string) (bool, error) {
	from, err := parseMinutes(start)
	if err != nil {
		return false, err
	}
	to, err := parseMinutes(end)
	if err != nil {
		return false, err
	}
	return inWindow(minutes, from, to), nil
}

func Evaluate(rule Rule, price Price, baseline *Price, now time.Time, lastDispatched *time.Time) (bool, []string) {
	reasons := []string{}
	loc, err := time.LoadLocation(rule.Schedule.Timezone)
	if err != nil {
		return false, []string{"invalid-timezone"}
	}
	local := now.In(loc)
	weekday := int(local.Weekday())
	dayOK := false
	for _, d := range rule.Schedule.Days {
		if d == weekday {
			dayOK = true
		}
	}
	if !dayOK {
		reasons = append(reasons, "wrong-day")
	}
	windowStart, _ := parseMinutes(rule.Schedule.WindowStart)
	windowEnd, _ := parseMinutes(rule.Schedule.WindowEnd)
	minutes := local.Hour()*60 + local.Minute()
	if !inWindow(minutes, windowStart, windowEnd) {
		reasons = append(reasons, "outside-window")
	}
	quietStart, _ := parseMinutes(rule.Schedule.QuietStart)
	quietEnd, _ := parseMinutes(rule.Schedule.QuietEnd)
	if quietStart != quietEnd && inWindow(minutes, quietStart, quietEnd) {
		reasons = append(reasons, "quiet-hours")
	}
	if lastDispatched != nil && rule.Schedule.CooldownHours > 0 {
		if now.Sub(*lastDispatched) < time.Duration(rule.Schedule.CooldownHours)*time.Hour {
			reasons = append(reasons, "cooldown-active")
		}
	}
	matched := make([]bool, len(rule.Predicates))
	for i, p := range rule.Predicates {
		if p.Source != price.Source {
			matched[i] = false
			reasons = append(reasons, "incompatible-source")
			continue
		}
		if p.Fuel != price.Fuel || p.Unit != price.Unit {
			matched[i] = false
			reasons = append(reasons, "incompatible-product")
			continue
		}
		if (p.Condition == kernel.Loyalty) != (price.Condition == kernel.Loyalty) {
			matched[i] = false
			reasons = append(reasons, "incompatible-condition")
			continue
		}
		switch p.Kind {
		case Threshold:
			matched[i] = price.Milli <= p.ThresholdMilli
			if matched[i] {
				reasons = append(reasons, "threshold-met")
			}
		case Change:
			if baseline == nil {
				matched[i] = false
				reasons = append(reasons, "missing-baseline")
				continue
			}
			drop := baseline.Milli - price.Milli
			ok := false
			if p.DropMilli > 0 && drop >= p.DropMilli {
				ok = true
			}
			if p.DropPercentBP > 0 && baseline.Milli > 0 && drop*10000 >= baseline.Milli*p.DropPercentBP {
				ok = true
			}
			matched[i] = ok
			if ok {
				reasons = append(reasons, "change-met")
			}
		}
	}
	predicatesOK := false
	if rule.Operator == "AND" {
		predicatesOK = true
		for _, m := range matched {
			predicatesOK = predicatesOK && m
		}
	} else {
		for _, m := range matched {
			predicatesOK = predicatesOK || m
		}
	}
	if len(reasons) > 0 {
		hasPositive := false
		for _, reason := range reasons {
			if reason == "threshold-met" || reason == "change-met" {
				hasPositive = true
			}
		}
		_ = hasPositive
	}
	scheduleOK := true
	for _, reason := range reasons {
		switch reason {
		case "wrong-day", "outside-window", "quiet-hours", "cooldown-active", "invalid-timezone":
			scheduleOK = false
		}
	}
	return predicatesOK && scheduleOK, reasons
}
