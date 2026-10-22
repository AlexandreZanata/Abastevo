package domain

import (
	"testing"
)

const testEnforcementProfile = "anp-enforcement-xlsx-2019-v1"

func TestGroupEnforcementRecordsPreservesDistinctResultsInOneCase(t *testing.T) {
	observations := []EnforcementObservation{
		{RecordKey: "enforcement-source:aaa", DocumentNumber: "712508", DocumentDay: "2026-10-04"},
		{RecordKey: "enforcement-source:bbb", DocumentNumber: "712508", DocumentDay: "2026-10-04"},
		{RecordKey: "enforcement-source:ccc", DocumentNumber: "712509", DocumentDay: "2019-01-02"},
	}
	cases, counts := GroupEnforcementRecords(testEnforcementProfile, observations)
	if counts.Observations != 3 || counts.Cases != 2 {
		t.Fatal("counts", counts)
	}
	if len(cases) != 2 || len(cases[0].Observations) != 2 || cases[0].Basis != CaseBasisSameDocument {
		t.Fatal("distinct results merged or lost", cases)
	}
	if cases[0].Observations[0] != "enforcement-source:aaa" || cases[0].Observations[1] != "enforcement-source:bbb" {
		t.Fatal("observation order lost", cases[0])
	}
}

func TestGroupEnforcementRecordsTreatsRepeatedSnapshotAsNoNewCase(t *testing.T) {
	observations := []EnforcementObservation{
		{RecordKey: "enforcement-source:aaa", DocumentNumber: "712508", DocumentDay: "2026-10-04"},
		{RecordKey: "enforcement-source:aaa", DocumentNumber: "712508", DocumentDay: "2026-10-04"},
	}
	_, counts := GroupEnforcementRecords(testEnforcementProfile, observations)
	if counts.Observations != 1 || counts.Cases != 1 {
		t.Fatal("repeated snapshot raised counts", counts)
	}
}

func TestGroupEnforcementRecordsKeepsCaseAcrossCorrection(t *testing.T) {
	before := []EnforcementObservation{
		{RecordKey: "enforcement-source:aaa", DocumentNumber: "712508", DocumentDay: "2026-10-04"},
	}
	after := []EnforcementObservation{
		{RecordKey: "enforcement-source:aaa", DocumentNumber: "712508", DocumentDay: "2026-10-04"},
		{RecordKey: "enforcement-source:zzz", DocumentNumber: "712508", DocumentDay: "2026-10-04"},
	}
	beforeCases, _ := GroupEnforcementRecords(testEnforcementProfile, before)
	afterCases, afterCounts := GroupEnforcementRecords(testEnforcementProfile, after)
	if len(beforeCases) != 1 || len(afterCases) != 1 || beforeCases[0].CaseKey != afterCases[0].CaseKey {
		t.Fatal("correction broke case stability")
	}
	if afterCounts.Observations != 2 || len(afterCases[0].Observations) != 2 {
		t.Fatal("corrected observation lost", afterCounts)
	}
}

func TestGroupEnforcementRecordsLeavesMissingDateAndDocumentUnknown(t *testing.T) {
	observations := []EnforcementObservation{
		{RecordKey: "enforcement-source:nodate", DocumentNumber: "712510", DocumentDay: ""},
		{RecordKey: "enforcement-source:nodoc", DocumentNumber: "", DocumentDay: "2026-10-04"},
	}
	cases, counts := GroupEnforcementRecords(testEnforcementProfile, observations)
	if counts.Observations != 2 || counts.Cases != 1 {
		t.Fatal("observations without date or document lost", counts)
	}
	if DateBasis("") != DateBasisUnknown || DateBasis("2026-10-04") != DateBasisDocumentDay || DateBasis("2026-13-40") != DateBasisUnknown {
		t.Fatal("date basis guessed")
	}
	if len(cases) != 1 || cases[0].DocumentNumber != "712510" {
		t.Fatal("undated observation dropped from its case", cases)
	}
}

func TestStableCaseKeyIsDeterministicAndScoped(t *testing.T) {
	first, ok := StableCaseKey(testEnforcementProfile, "712508")
	second, ok2 := StableCaseKey(testEnforcementProfile, "712508")
	other, ok3 := StableCaseKey(testEnforcementProfile, "712509")
	if !ok || !ok2 || !ok3 || first != second || first == other {
		t.Fatal("case identity unstable or colliding")
	}
	if _, ok := StableCaseKey(testEnforcementProfile, ""); ok {
		t.Fatal("empty document formed a case")
	}
	if _, ok := StableCaseKey("", "712508"); ok {
		t.Fatal("empty profile formed a case")
	}
}
