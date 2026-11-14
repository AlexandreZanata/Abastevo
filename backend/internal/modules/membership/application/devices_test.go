package application

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

const deviceTestID = "7b0e2a54-0000-4000-8000-000000003101"
const deviceTestMember = "7b0e2a54-0000-4000-8000-000000003102"
const deviceTestEnrollment = "7b0e2a54-0000-4000-8000-000000003103"

func deviceTestScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, e := kernel.ParseOrgID(deviceTestID)
	if e != nil {
		t.Fatal(e)
	}
	scope, e := kernel.NewOrgScope(id)
	if e != nil {
		t.Fatal(e)
	}
	return scope
}

func deviceTestActor() DeviceActor {
	return DeviceActor{AccountID: deviceTestID, FamilyID: deviceTestMember, ContributorID: deviceTestEnrollment, KeyID: "7b0e2a54-0000-4000-8000-000000003104", Fingerprint: "fp:" + strings.Repeat("a", 64), AccessToken: "synthetic-private-device-access", ProofExpires: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)}
}

func deviceTestRecord(t *testing.T, scope kernel.OrgScope, revoked bool) DeviceRecord {
	t.Helper()
	state := d.DeviceSnapshot{OrgID: scope.OrgID(), ID: deviceTestEnrollment, MembershipID: deviceTestMember, DeviceID: deviceTestID, Fingerprint: "fp:" + strings.Repeat("a", 64), State: d.DeviceEnrolled, Revision: 1}
	if revoked {
		state.State, state.Revision, state.Revocation = d.DeviceRevoked, 2, d.LostOrCompromised
	}
	enrollment, e := d.RehydrateDevice(scope, state)
	if e != nil {
		t.Fatal(e)
	}
	record := DeviceRecord{Enrollment: enrollment, Label: "Synthetic private device", EnrolledAt: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)}
	if revoked {
		at := record.EnrolledAt.Add(time.Minute)
		record.RevokedAt = &at
		record.SourceType, record.SourceID, record.SourceRevision = "COMMAND", deviceTestID, 1
	}
	return record
}

func TestDeviceActorRequiresCompleteProofMetadataAndRedactsSecrets(t *testing.T) {
	actor := deviceTestActor()
	if e := actor.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*DeviceActor){
		func(v *DeviceActor) { v.AccountID = "" },
		func(v *DeviceActor) { v.FamilyID = "foreign" },
		func(v *DeviceActor) { v.ContributorID = "00000000-0000-0000-0000-000000000000" },
		func(v *DeviceActor) { v.KeyID = strings.ToUpper(v.KeyID) },
		func(v *DeviceActor) { v.Fingerprint = "fp:" + strings.Repeat("A", 64) },
		func(v *DeviceActor) { v.Fingerprint = "a" },
		func(v *DeviceActor) { v.AccessToken = "" },
		func(v *DeviceActor) { v.AccessToken = strings.Repeat("s", 513) },
		func(v *DeviceActor) { v.ProofExpires = time.Time{} },
	} {
		invalid := actor
		mutate(&invalid)
		if e := invalid.Validate(); e != d.ErrInvalid {
			t.Fatal("incomplete device proof metadata accepted", e)
		}
	}
	raw, e := json.Marshal(actor)
	if e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{fmt.Sprintf("%+v", actor), string(raw), actor.LogValue().String()} {
		if strings.Contains(value, actor.AccessToken) || strings.Contains(value, actor.Fingerprint) || strings.Contains(value, actor.FamilyID) {
			t.Fatal("device actor disclosed private proof")
		}
	}
}

func TestDeviceCommandsBindExactOrganizationIntentWithoutAuthority(t *testing.T) {
	scope := deviceTestScope(t)
	enroll := DeviceCommand{OperationID: deviceTestID, Label: "Synthetic private device"}
	revoke := DeviceCommand{OperationID: deviceTestID, DeviceID: deviceTestEnrollment, ExpectedRevision: 1, Class: d.LostOrCompromised, Reason: "  Synthetic exact reason  "}
	for _, c := range []struct {
		value  DeviceCommand
		enroll bool
	}{{enroll, true}, {revoke, false}} {
		if e := c.value.Validate(c.enroll); e != nil || len(c.value.Digest(scope, c.enroll)) != 64 {
			t.Fatal("valid device intent", e)
		}
		if c.value.Digest(kernel.OrgScope{}, c.enroll) != "" || c.value.Digest(scope, !c.enroll) != "" {
			t.Fatal("unbound device digest")
		}
	}
	for _, mutate := range []func(*DeviceCommand){
		func(v *DeviceCommand) { v.OperationID = "not-a-uuid" },
		func(v *DeviceCommand) { v.DeviceID = deviceTestEnrollment },
		func(v *DeviceCommand) { v.ExpectedRevision = 1 },
		func(v *DeviceCommand) { v.Class = d.AccessEnded },
		func(v *DeviceCommand) { v.Reason = "Synthetic unwanted reason" },
		func(v *DeviceCommand) { v.Label = " Synthetic private device" },
		func(v *DeviceCommand) { v.Label = "" },
	} {
		invalid := enroll
		mutate(&invalid)
		if invalid.Validate(true) != d.ErrInvalid || invalid.Digest(scope, true) != "" {
			t.Fatal("invalid enrolment intent accepted")
		}
	}
	for _, mutate := range []func(*DeviceCommand){
		func(v *DeviceCommand) { v.OperationID = "" },
		func(v *DeviceCommand) { v.DeviceID = "foreign" },
		func(v *DeviceCommand) { v.ExpectedRevision = 0 },
		func(v *DeviceCommand) { v.ExpectedRevision = math.MaxInt64 },
		func(v *DeviceCommand) { v.Label = "Synthetic unexpected label" },
		func(v *DeviceCommand) { v.Class = "unknown" },
		func(v *DeviceCommand) { v.Reason = " " },
		func(v *DeviceCommand) { v.Reason = "Synthetic\u200breason" },
	} {
		invalid := revoke
		mutate(&invalid)
		if invalid.Validate(false) != d.ErrInvalid || invalid.Digest(scope, false) != "" {
			t.Fatal("invalid revocation intent accepted")
		}
	}
	for _, mutate := range []func(*DeviceCommand){
		func(v *DeviceCommand) { v.OperationID = deviceTestMember },
		func(v *DeviceCommand) { v.DeviceID = deviceTestMember },
		func(v *DeviceCommand) { v.ExpectedRevision = 2 },
		func(v *DeviceCommand) { v.Class = d.AccessEnded },
		func(v *DeviceCommand) { v.Reason = strings.TrimSpace(v.Reason) },
	} {
		changed := revoke
		mutate(&changed)
		if changed.Digest(scope, false) == revoke.Digest(scope, false) {
			t.Fatal("changed revocation intent reused original digest")
		}
	}
	otherID, _ := kernel.ParseOrgID(deviceTestMember)
	other, _ := kernel.NewOrgScope(otherID)
	if revoke.Digest(scope, false) == revoke.Digest(other, false) {
		t.Fatal("device command digest crossed organization boundary")
	}
}

func TestDeviceRecordsRequireHonestSyncAndRevocationSources(t *testing.T) {
	scope := deviceTestScope(t)
	enrolled, revoked := deviceTestRecord(t, scope, false), deviceTestRecord(t, scope, true)
	if enrolled.Validate(scope) != nil || revoked.Validate(scope) != nil {
		t.Fatal("valid device record")
	}
	for _, mutate := range []func(*DeviceRecord){
		func(v *DeviceRecord) { v.Label = "" },
		func(v *DeviceRecord) { v.EnrolledAt = time.Time{} },
		func(v *DeviceRecord) { at := v.EnrolledAt; v.LastSyncAt = &at },
		func(v *DeviceRecord) { at := v.EnrolledAt; v.RevokedAt = &at },
		func(v *DeviceRecord) { v.SourceType = "COMMAND" },
		func(v *DeviceRecord) { v.SourceID = deviceTestID },
		func(v *DeviceRecord) { v.SourceRevision = 1 },
	} {
		invalid := enrolled
		mutate(&invalid)
		if invalid.Validate(scope) == nil {
			t.Fatal("invented enrolment metadata accepted")
		}
	}
	for _, mutate := range []func(*DeviceRecord){
		func(v *DeviceRecord) { v.RevokedAt = nil },
		func(v *DeviceRecord) { at := time.Time{}; v.RevokedAt = &at },
		func(v *DeviceRecord) { at := v.EnrolledAt.Add(-time.Second); v.RevokedAt = &at },
		func(v *DeviceRecord) { v.SourceType = "" },
		func(v *DeviceRecord) { v.SourceType = "fabricated" },
		func(v *DeviceRecord) { v.SourceID = "foreign" },
		func(v *DeviceRecord) { v.SourceRevision = 2 },
	} {
		invalid := revoked
		mutate(&invalid)
		if invalid.Validate(scope) == nil {
			t.Fatal("invalid device revocation source accepted")
		}
	}
	state, _ := revoked.Enrollment.Snapshot(scope)
	state.Revocation = d.AccessEnded
	revoked.Enrollment, _ = d.RehydrateDevice(scope, state)
	revoked.SourceType, revoked.SourceID, revoked.SourceRevision = "MEMBERSHIP", deviceTestMember, 4
	if e := revoked.Validate(scope); e != nil {
		t.Fatal("actual ended-membership source", e)
	}
	for _, mutate := range []func(*DeviceRecord){
		func(v *DeviceRecord) { v.SourceID = deviceTestID },
		func(v *DeviceRecord) { v.SourceRevision = 0 },
		func(v *DeviceRecord) {
			state.Revocation = d.LostOrCompromised
			v.Enrollment, _ = d.RehydrateDevice(scope, state)
		},
	} {
		invalid := revoked
		mutate(&invalid)
		if invalid.Validate(scope) == nil {
			t.Fatal("membership source invented a revocation")
		}
	}
	otherID, _ := kernel.ParseOrgID(deviceTestMember)
	other, _ := kernel.NewOrgScope(otherID)
	if enrolled.Validate(other) == nil || (DeviceRecord{}).Validate(scope) == nil {
		t.Fatal("foreign or missing enrolment accepted")
	}
}

func TestDevicePolicyKeepsFreshScopedManagementAndReduction(t *testing.T) {
	for _, write := range []bool{false, true} {
		policy := DevicePolicy(write)
		want := access.Read
		if write {
			want = access.ReduceAccess
		}
		if e := policy.Validate(); e != nil || policy.Permission != "idn.credential.manage" || policy.Capability != "organizations" || !policy.Fresh || policy.Effect != want {
			t.Fatal("device policy", e)
		}
	}
}

func TestDeviceValuesRequireExplicitPrivateBoundaryMapping(t *testing.T) {
	scope := deviceTestScope(t)
	record := deviceTestRecord(t, scope, true)
	command := DeviceCommand{OperationID: deviceTestID, DeviceID: deviceTestEnrollment, ExpectedRevision: 1, Class: d.LostOrCompromised, Reason: "Synthetic private revocation reason"}
	result := DeviceResult{Device: record, OperationID: command.OperationID, Reason: command.Reason}
	for _, value := range []any{record, command, result, DevicePage{Devices: []DeviceRecord{record}, NextID: deviceTestEnrollment}} {
		raw, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		serialized := fmt.Sprintf("%+v %s", value, raw)
		for _, private := range []string{record.Label, command.Reason, command.DeviceID, command.OperationID} {
			if strings.Contains(serialized, private) {
				t.Fatal("implicit device serialization disclosed private metadata")
			}
		}
	}
}
