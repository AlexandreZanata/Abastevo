package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validReportCommand() ReportCommand {
	return ReportCommand{CommandID: "11111111-1111-4111-8111-111111111111", ReportID: "22222222-2222-4222-8222-222222222222", Action: "SUBMIT", Kind: "EXPERIENCE", StationID: "33333333-3333-4333-8333-333333333333", Fuel: "ETHANOL", OccurredOn: "2026-10-13", Symptom: "OTHER", Text: "Observed difficulty starting after refuelling."}
}

func TestReportValidation(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		change func(*ReportCommand)
	}{
		{"command identity", func(c *ReportCommand) { c.CommandID = "x" }},
		{"report identity", func(c *ReportCommand) { c.ReportID = "x" }},
		{"station identity", func(c *ReportCommand) { c.StationID = "x" }},
		{"fuel", func(c *ReportCommand) { c.Fuel = "" }},
		{"future", func(c *ReportCommand) { c.OccurredOn = "2026-10-15" }},
		{"invalid day", func(c *ReportCommand) { c.OccurredOn = "2026-02-30" }},
		{"empty", func(c *ReportCommand) { c.Text = "  " }},
		{"text limit", func(c *ReportCommand) { c.Text = strings.Repeat("a", 1001) }},
		{"encoding", func(c *ReportCommand) { c.Text = string([]byte{0xff}) }},
		{"control", func(c *ReportCommand) { c.Text = "private\x00text" }},
		{"symptom", func(c *ReportCommand) { c.Symptom = "CONFIRMED_ADULTERATION" }},
		{"verification", func(c *ReportCommand) { c.Kind = "PURCHASE_VERIFIED" }},
		{"partial target", func(c *ReportCommand) { c.RevisionID = "44444444-4444-4444-8444-444444444444" }},
		{"unrelated review", func(c *ReportCommand) { c.Kind = "LINK_REVIEW" }},
		{"edit revision", func(c *ReportCommand) { c.Action = "EDIT" }},
		{"withdraw payload", func(c *ReportCommand) { c.Action = "WITHDRAW"; c.ExpectedRevision = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := validReportCommand()
			tc.change(&c)
			if !errors.Is(c.Validate(now), ErrReportInvalid) {
				t.Fatal("invalid command accepted")
			}
		})
	}
	c := validReportCommand()
	if err := c.Validate(now); err != nil {
		t.Fatal(err)
	}
	c.Action = "EDIT"
	c.ExpectedRevision = 1
	if err := c.Validate(now); err != nil {
		t.Fatal(err)
	}
	c = ReportCommand{CommandID: c.CommandID, ReportID: c.ReportID, Action: "WITHDRAW", ExpectedRevision: 2}
	if err := c.Validate(now); err != nil {
		t.Fatal(err)
	}
	c = validReportCommand()
	c.Kind = "LINK_REVIEW"
	c.RevisionID = "44444444-4444-4444-8444-444444444444"
	c.SampleRef = "sample-1"
	if err := c.Validate(now); err != nil {
		t.Fatal(err)
	}
}

func TestReportCommandHashBindsEveryField(t *testing.T) {
	c := validReportCommand()
	h := c.Hash()
	if h != c.Hash() {
		t.Fatal("unstable")
	}
	c.Text += "Changed"
	if h == c.Hash() {
		t.Fatal("body not bound")
	}
	c = validReportCommand()
	c.ExpectedRevision = 2
	if h == c.Hash() {
		t.Fatal("revision not bound")
	}
}
