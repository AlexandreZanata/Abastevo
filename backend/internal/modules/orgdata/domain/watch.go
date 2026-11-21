package domain

import (
	"math/big"
	"time"
)

type WatchMove string

const (
	WatchUnavailable WatchMove = "UNAVAILABLE"
	WatchUnchanged   WatchMove = "UNCHANGED"
	WatchIncrease    WatchMove = "INCREASE"
	WatchDecrease    WatchMove = "DECREASE"
)

type WatchObservation struct {
	Revision                  int64
	Region, Method, SurveyEnd string
	MinimumStations           int
	Sample                    Sample
}

// CompareWatchObservations compares the exact station means of successive
// accepted observations. The source reader proves each revision's admission.
func CompareWatchObservations(before, next WatchObservation) WatchMove {
	beforeDay, beforeErr := time.Parse(surveyDate, before.SurveyEnd)
	nextDay, nextErr := time.Parse(surveyDate, next.SurveyEnd)
	if beforeErr != nil || nextErr != nil || !nextDay.After(beforeDay) ||
		before.Revision < 1 || next.Revision <= before.Revision || !marketState(before.Region) ||
		before.Region != next.Region || before.Method == "" || before.Method != next.Method ||
		before.MinimumStations < 5 || next.MinimumStations < 5 ||
		before.Sample.Fuel != next.Sample.Fuel || before.Sample.Unit != next.Sample.Unit ||
		!before.Sample.Valid(before.MinimumStations) || !next.Sample.Valid(next.MinimumStations) {
		return WatchUnavailable
	}
	beforeMean := big.NewRat(before.Sample.SumMilliBRL, int64(before.Sample.Stations))
	nextMean := big.NewRat(next.Sample.SumMilliBRL, int64(next.Sample.Stations))
	switch nextMean.Cmp(beforeMean) {
	case 1:
		return WatchIncrease
	case -1:
		return WatchDecrease
	default:
		return WatchUnchanged
	}
}
