package domain_test

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
)

func scope(t *testing.T, id string) kernel.OrgScope {
	t.Helper()
	value, err := kernel.ParseOrgID(id)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(value)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const own = "7b0e2a54-0000-4000-8000-000000000201"
const other = "7b0e2a54-0000-4000-8000-000000000202"
const employeeID = "7b0e2a54-0000-4000-8000-000000000203"
const memberID = "7b0e2a54-0000-4000-8000-000000000204"
const accountID = "7b0e2a54-0000-4000-8000-000000000205"
const deviceID = "7b0e2a54-0000-4000-8000-000000000206"
const enrollmentID = "7b0e2a54-0000-4000-8000-000000000207"

func employee(t *testing.T, s kernel.OrgScope) d.Employee {
	t.Helper()
	v, err := d.NewEmployee(s, employeeID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func account(t *testing.T) d.AccountRef {
	t.Helper()
	a, err := d.PersonalAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func member(t *testing.T, s kernel.OrgScope, state d.MembershipState) d.Membership {
	t.Helper()
	snap := d.MembershipSnapshot{OrgID: s.OrgID(), ID: memberID, EmployeeID: employeeID, Account: account(t), Model: d.ExistingAccount, State: state, Revision: 1}
	if state == d.MembershipSuspended {
		snap.Suspension = d.ByManager
	}
	if state == d.MembershipEnded {
		snap.EndReason = d.Offboarded
	}
	v, err := d.RehydrateMembership(s, snap)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func checks(t *testing.T, s kernel.OrgScope) d.MembershipConditions {
	return d.MembershipConditions{Employee: employee(t, s), AdmissionConfirmed: true, ConfirmationSatisfied: true, LiveRoleAssignments: 1, OwnershipProtected: true, ResponsibilitiesReviewed: true, ReasonRecorded: true, Suspension: d.ByManager, EndReason: d.Offboarded}
}
func code(err error) string {
	switch err {
	case nil:
		return "ok"
	case d.ErrTransition:
		return "transition"
	case d.ErrEnded:
		return "ended"
	case d.ErrRevoked:
		return "revoked"
	}
	return "unexpected"
}
func TestLifecycleVectors(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/enterprise/identity-lifecycle-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	if !scan.Scan() || scan.Text() != "id|entity|from|to|expected" {
		t.Fatal("fixture header")
	}
	ids := map[string]bool{}
	states := map[string]string{"employee": "|ACTIVE|INACTIVE|TERMINATED|", "membership": "|INVITED|ACTIVE|SUSPENDED|ENDED|", "device": "|ENROLLED|REVOKED|"}
	s := scope(t, own)
	for scan.Scan() {
		fields := strings.Split(scan.Text(), "|")
		if len(fields) != 5 || ids[fields[0]] {
			t.Fatal("malformed/duplicate vector")
		}
		if fields[0] != fields[1]+"-"+fields[2]+"-"+fields[3] ||
			!strings.Contains(states[fields[1]], "|"+fields[2]+"|") ||
			!strings.Contains(states[fields[1]], "|"+fields[3]+"|") {
			t.Fatal("unknown state or inconsistent vector ID")
		}
		ids[fields[0]] = true
		t.Run(fields[0], func(t *testing.T) {
			var err error
			switch fields[1] {
			case "employee":
				before, e := d.RehydrateEmployee(s, d.EmployeeSnapshot{OrgID: s.OrgID(), ID: employeeID, State: d.EmployeeState(fields[2]), Revision: 1})
				if e != nil {
					t.Fatal(e)
				}
				after, e := before.Transition(s, 1, d.EmployeeState(fields[3]), d.EmployeeConditions{OwnershipProtected: true, ResponsibilitiesReviewed: true, ReasonRecorded: true})
				err = e
				if err == nil {
					snap, e := after.Snapshot(s)
					if e != nil || string(snap.State) != fields[3] || snap.ID != employeeID || snap.Revision != 2 {
						t.Fatal("employee result", e)
					}
				}
				snap, _ := before.Snapshot(s)
				if string(snap.State) != fields[2] || snap.Revision != 1 {
					t.Fatal("mutated employee")
				}
			case "membership":
				before := member(t, s, d.MembershipState(fields[2]))
				after, e := before.Transition(s, 1, d.MembershipState(fields[3]), checks(t, s))
				err = e
				if err == nil {
					snap, e := after.Snapshot(s)
					if e != nil || string(snap.State) != fields[3] || snap.ID != memberID || snap.Revision != 2 {
						t.Fatal("membership result", e)
					}
				}
				snap, _ := before.Snapshot(s)
				if string(snap.State) != fields[2] || snap.Revision != 1 {
					t.Fatal("mutated membership")
				}
			case "device":
				snap := d.DeviceSnapshot{OrgID: s.OrgID(), ID: enrollmentID, MembershipID: memberID, DeviceID: deviceID, Fingerprint: "fp:" + strings.Repeat("a", 64), State: d.DeviceState(fields[2]), Revision: 1}
				if snap.State == d.DeviceRevoked {
					snap.Revocation = d.AccessEnded
				}
				before, e := d.RehydrateDevice(s, snap)
				if e != nil {
					t.Fatal(e)
				}
				after, e := before.Transition(s, 1, d.DeviceState(fields[3]), d.LostOrCompromised)
				err = e
				if err == nil {
					snap, e := after.Snapshot(s)
					if e != nil || string(snap.State) != fields[3] || snap.ID != enrollmentID || snap.Revision != 2 || snap.Revocation != d.LostOrCompromised {
						t.Fatal("device result", e)
					}
				}
				snap, e = before.Snapshot(s)
				if e != nil || string(snap.State) != fields[2] || snap.Revision != 1 {
					t.Fatal("mutated device")
				}
			default:
				t.Fatal("unknown fixture entity")
			}
			if got := code(err); got != fields[4] {
				t.Fatalf("result %s, want %s: %v", got, fields[4], err)
			}
		})
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 29 {
		t.Fatalf("got %d vectors, want 29", len(ids))
	}
}
