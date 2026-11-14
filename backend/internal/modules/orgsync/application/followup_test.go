package application_test

import (
	"errors"
	"slices"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

func TestDescriptorFreezesExplicitTerminalResults(t *testing.T) {
	h := newHandler()
	h.descriptor.TerminalResults = []string{protocol.Accepted, protocol.Rejected}
	registry, err := app.NewRegistry(h)
	if err != nil {
		t.Fatal("valid explicit terminal set refused", err)
	}
	h.descriptor.TerminalResults[0] = protocol.AcceptedForReview
	frozen, ok := registry.Lookup("synthetic.record")
	if !ok || !slices.Equal(frozen.Descriptor().Results(), []string{protocol.Accepted, protocol.Rejected}) {
		t.Fatal("registered terminal results changed or implicit review was added")
	}
	view := frozen.Descriptor()
	view.TerminalResults[0] = protocol.Retry
	view = frozen.Descriptor().Copy()
	view.TerminalResults[0] = protocol.Retry
	results := frozen.Descriptor().Results()
	results[0] = protocol.Retry
	if !slices.Equal(frozen.Descriptor().Results(), []string{protocol.Accepted, protocol.Rejected}) {
		t.Fatal("returned terminal results mutate the registered source")
	}
}

func TestDescriptorRejectsInvalidTerminalDeclarations(t *testing.T) {
	for _, results := range [][]string{{protocol.Accepted}, {protocol.AcceptedForReview}, {protocol.Retry, protocol.Rejected}, {"UNKNOWN", protocol.Rejected}, {protocol.Rejected, protocol.Rejected}, {"", protocol.Rejected}} {
		h := newHandler()
		h.descriptor.TerminalResults = results
		if !errors.Is(h.descriptor.Validate(), app.ErrUnavailable) {
			t.Fatal("invalid terminal declaration accepted")
		}
		if _, err := app.NewRegistry(h); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid terminal declaration registered")
		}
	}
	for _, results := range [][]string{nil, {}, {protocol.Rejected}, {protocol.Accepted, protocol.Rejected}, {protocol.Accepted, protocol.AcceptedForReview, protocol.Rejected}} {
		h := newHandler()
		h.descriptor.TerminalResults = results
		if err := h.descriptor.Validate(); err != nil {
			t.Fatal("valid declared/default terminal set refused", err)
		}
	}
	if !slices.Equal(newHandler().Descriptor().Results(), []string{protocol.Accepted, protocol.AcceptedForReview, protocol.Rejected}) {
		t.Fatal("legacy descriptor lost its required followups")
	}
}
