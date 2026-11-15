package domain_test

import (
	"errors"
	"strings"
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
)

// Every row of fleet.md §6.1 owned by EMP-FLEET-003, plus the state-preserving
// acquisition record. Anything absent from this table must be refused.
var lifecycleTable = map[fleet.LifecycleCommand]map[fleet.State]fleet.State{
	fleet.RecordAcquisition: {fleet.Draft: fleet.Draft, fleet.Active: fleet.Active, fleet.Inactive: fleet.Inactive},
	fleet.Deactivate:        {fleet.Active: fleet.Inactive},
	fleet.Reactivate:        {fleet.Inactive: fleet.Active},
	fleet.Dispose:           {fleet.Active: fleet.Disposed, fleet.Inactive: fleet.Disposed},
	fleet.RevertDisposal:    {fleet.Disposed: fleet.Inactive},
	fleet.Archive:           {fleet.Disposed: fleet.Archived},
}

func TestLifecycleTableTransitionsAndRefusalsForEveryRow(t *testing.T) {
	states := []fleet.State{fleet.Draft, fleet.Active, fleet.Inactive, fleet.Disposed, fleet.Archived}
	if len(fleet.LifecycleCommands()) != len(lifecycleTable) {
		t.Fatal("lifecycle command list drifted from the specification table")
	}
	for _, command := range fleet.LifecycleCommands() {
		rows, ok := lifecycleTable[command]
		if !ok {
			t.Fatal("unknown lifecycle command", command)
		}
		for _, from := range states {
			t.Run(string(command)+"/"+string(from), func(t *testing.T) {
				got, err := fleet.Transition(from, command)
				if want, allowed := rows[from]; allowed {
					if err != nil || got != want {
						t.Fatalf("transition %s from %s: got %q %v want %q", command, from, got, err, want)
					}
					return
				}
				if !errors.Is(err, fleet.ErrTransition) || got != "" {
					t.Fatalf("refusal %s from %s: got %q %v", command, from, got, err)
				}
			})
		}
	}
}

func TestLifecycleRefusesUnknownCommandsStatesAndTerminalArchive(t *testing.T) {
	for _, command := range fleet.LifecycleCommands() {
		if _, err := fleet.Transition(fleet.Archived, command); !errors.Is(err, fleet.ErrTransition) {
			t.Fatal("ARCHIVED is not terminal for", command)
		}
		if _, err := fleet.Transition("UNKNOWN", command); !errors.Is(err, fleet.ErrTransition) {
			t.Fatal("unknown source state accepted")
		}
	}
	for _, command := range []fleet.LifecycleCommand{"", "ACTIVATE", "DISCARD", "deactivate", "SELL"} {
		if _, err := fleet.Transition(fleet.Active, command); !errors.Is(err, fleet.ErrTransition) {
			t.Fatal("unknown lifecycle command accepted", command)
		}
	}
}

func TestLifecycleReasonAndOpenWorkRowsFollowSpecification(t *testing.T) {
	reason := map[fleet.LifecycleCommand]bool{fleet.Deactivate: true, fleet.Dispose: true, fleet.RevertDisposal: true}
	openWork := map[fleet.LifecycleCommand]bool{fleet.Deactivate: true, fleet.Dispose: true, fleet.Archive: true}
	for _, command := range fleet.LifecycleCommands() {
		if fleet.RequiresReason(command) != reason[command] {
			t.Fatal("reason row drifted", command)
		}
		if fleet.ChecksOpenWork(command) != openWork[command] {
			t.Fatal("BR-FLEET-010 row drifted", command)
		}
	}
	if fleet.RequiresReason("UNKNOWN") || fleet.ChecksOpenWork("UNKNOWN") {
		t.Fatal("unknown command gained a row")
	}
}

func TestDisposalKindIsClosedAndExact(t *testing.T) {
	for _, raw := range []string{"SALE", "WRITE_OFF", "RETURN_TO_HOLDER"} {
		if kind, err := fleet.ParseDisposalKind(raw); err != nil || string(kind) != raw {
			t.Fatal("disposal kind refused", raw, err)
		}
	}
	for _, raw := range []string{"", "sale", "Sale", "RETURN", "THEFT", "SALE ", " WRITE_OFF", "WRITE-OFF"} {
		if _, err := fleet.ParseDisposalKind(raw); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("unknown disposal kind accepted", raw)
		}
	}
}

func TestLifecycleReasonUsesTheSharedFreeTextBound(t *testing.T) {
	for _, raw := range []string{"x", "Vendido ao comprador sintético", strings.Repeat("ã", 280), "Baixa: sinistro total é"} {
		if !fleet.ValidReason(raw) {
			t.Fatal("valid reason refused", len(raw))
		}
	}
	for _, raw := range []string{"", " ", "  ", strings.Repeat("a", 281), strings.Repeat("ã", 281), "line\nbreak", "tab\there", "nul\x00", "bell\u0085", string([]byte{0xff, 'a'})} {
		if fleet.ValidReason(raw) {
			t.Fatal("invalid reason accepted", raw)
		}
	}
}
