package domain

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SourceStaleAfterDays is the existing FQ-08 age rule shared by revision and source clocks.
const SourceStaleAfterDays = 180

var fineAmountShape = regexp.MustCompile(`^(0|[1-9][0-9]*|[1-9][0-9]{0,2}(\.[0-9]{3})+),[0-9]{2}$`)

// ParseFineAmount preserves exact source minor units without floating point,
// balances or totals. Both observed ungrouped and strict grouped forms are valid.
func ParseFineAmount(text string) (int64, bool) {
	if !fineAmountShape.MatchString(text) {
		return 0, false
	}
	value, e := strconv.ParseInt(strings.NewReplacer(".", "", ",", "").Replace(text), 10, 64)
	return value, e == nil && value >= 0
}

// FineMonthStale uses the earliest possible source day only as an age bound.
// It never claims this was the extraction day. Uncertain ages fail conservative.
func FineMonthStale(month string, now time.Time) (bool, string) {
	start, e := time.Parse("2006-01", month)
	if e != nil || start.Format("2006-01") != month || start.Year() < 1 {
		return true, "source extraction month unavailable"
	}
	if now.Before(start) {
		return true, "source extraction month is in the future"
	}
	if now.Sub(start) > time.Duration(SourceStaleAfterDays)*24*time.Hour {
		return true, "source extraction month may exceed 180 days"
	}
	return false, ""
}

// FineSourceClockStale uses a source day/month only as an age bound. UTC midnight
// for DAY is the earliest possible day boundary, never an invented source instant.
func FineSourceClockStale(precision, value string, now time.Time) (bool, string) {
	if precision == "MONTH" {
		return FineMonthStale(value, now)
	}
	if precision != "DAY" || !ValidDay(value) {
		return true, "source extraction clock unavailable"
	}
	start, _ := time.Parse("2006-01-02", value)
	if now.Before(start) {
		return true, "source extraction day is in the future"
	}
	if now.Sub(start) > time.Duration(SourceStaleAfterDays)*24*time.Hour {
		return true, "source extraction day exceeds 180 days"
	}
	return false, ""
}
