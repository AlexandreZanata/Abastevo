package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// EDR-284: the run owner composes only from real ports and has no held path.
func TestRunHandlerNeedsEveryPortAndHasNoHeldPath(t *testing.T) {
	if _, err := NewRunHandler(nil, nil, nil, nil); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("handler composed without ports", err)
	}
	var store *RunStore
	if _, err := NewRunHandler(nil, nil, nil, store); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("handler composed with a nil store", err)
	}
	d := (&runHandler{}).Descriptor()
	if d.Validate() != nil || d.Type != "check.run.complete" || d.Schema != 1 || len(d.Results()) != 2 || d.Results()[0] != protocol.Accepted || d.Results()[1] != protocol.Rejected ||
		d.WritePolicy != app.RunPolicy(app.RunWrite) || d.ReadPolicy != app.RunPolicy(app.RunReceipt) || d.WritePolicy.Permission != "check.checklist.execute" {
		t.Fatal("descriptor drift")
	}
}

// One release audit action on the checklist_run target (BR-AUD-004: no reason).
func TestRunAuditActionIsTheRelease(t *testing.T) {
	definitions := RunAuditDefinitions()
	if len(definitions) != 1 || definitions[0].Action != "check.run.released" || definitions[0].TargetType != "checklist_run" {
		t.Fatal("run audit definitions", definitions)
	}
	if _, err := audit.NewCatalog(definitions); err != nil {
		t.Fatal(err)
	}
}

// The intake store and the block source refuse to run without a transaction.
func TestRunStoreIntakeAndBlockSourceRefuseWithoutTransaction(t *testing.T) {
	s := &RunStore{}
	id := "9c000000-0000-4000-8000-000000002001"
	if _, err := s.RunOperationTx(context.Background(), nil, kernel.OrgScope{}, id); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("operation lookup without a transaction", err)
	}
	if _, err := s.RunTemplateTx(context.Background(), nil, kernel.OrgScope{}, id, 1, id); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("template read without a transaction", err)
	}
	if _, err := s.ReadRunTx(context.Background(), nil, kernel.OrgScope{}, id); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("run read without a transaction", err)
	}
	if _, err := (ChecklistBlocks{}).ChecklistBlocksDBTX(context.Background(), nil, kernel.OrgScope{}, id, time.Now()); !errors.Is(err, fleetapp.ErrUnavailable) {
		t.Fatal("block source without a transaction", err)
	}
	if _, err := s.ReadRun(context.Background(), kernel.OrgScope{}, app.IncidentActor{}, id, id, id); err == nil {
		t.Fatal("company read without scope")
	}
}
