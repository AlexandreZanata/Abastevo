package domain_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
)

func TestEmployeeExistsWithoutProvisioningAccessAndRehireKeepsPeriodsEnded(t *testing.T) {
	s := scope(t, own)
	e := employee(t, s)
	first, err := e.Snapshot(s)
	if err != nil || first.State != d.EmployeeActive || first.Revision != 1 || first.MergedInto != "" {
		t.Fatal("employee identity", err)
	}
	profile, err := d.DriverProfile(s, e)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := profile.EmployeeID(s); err != nil || id != employeeID {
		t.Fatal("optional driver association", err)
	}
	m, err := d.NewInvited(s, memberID, e, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	invited, _ := m.Snapshot(s)
	if invited.Account != (d.AccountRef{}) || invited.State != d.MembershipInvited {
		t.Fatal("employee created account/access")
	}
	bound, err := m.BindAccount(s, 1, account(t))
	if err != nil {
		t.Fatal(err)
	}
	afterBind, _ := bound.Snapshot(s)
	if afterBind.State != d.MembershipInvited {
		t.Fatal("association activated access")
	}
	active, err := bound.Transition(s, 2, d.MembershipActive, checks(t, s))
	if err != nil {
		t.Fatal(err)
	}
	ended, err := active.Transition(s, 3, d.MembershipEnded, checks(t, s))
	if err != nil {
		t.Fatal(err)
	}
	historical, _ := ended.Snapshot(s)
	if historical.ID != memberID || historical.EmployeeID != employeeID || historical.Account != account(t) || historical.EndReason != d.Offboarded {
		t.Fatal("ended period lost references")
	}
	terminated, err := e.Transition(s, 1, d.EmployeeTerminated, d.EmployeeConditions{OwnershipProtected: true, ResponsibilitiesReviewed: true})
	if err != nil {
		t.Fatal(err)
	}
	rehired, err := terminated.Transition(s, 2, d.EmployeeActive, d.EmployeeConditions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ended.Transition(s, 4, d.MembershipActive, checks(t, s)); err != d.ErrEnded {
		t.Fatal("rehire reopened ended period", err)
	}
	next, err := d.NewInvited(s, enrollmentID, rehired, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	nextSnap, _ := next.Snapshot(s)
	if nextSnap.ID == historical.ID || nextSnap.Account != (d.AccountRef{}) || nextSnap.State != d.MembershipInvited {
		t.Fatal("new period restored prior access")
	}
	unchanged, _ := e.Snapshot(s)
	if unchanged != first {
		t.Fatal("employee was mutated")
	}
	historicalAgain, _ := ended.Snapshot(s)
	if historicalAgain != historical {
		t.Fatal("ended history mutated")
	}
}

func TestAccountModelsAndOrganizationMembershipsStayIndependent(t *testing.T) {
	a, b := scope(t, own), scope(t, other)
	personal := account(t)
	for _, s := range []kernel.OrgScope{a, b} {
		e := employee(t, s)
		m, err := d.NewInvited(s, memberID, e, d.ExistingAccount)
		if err != nil {
			t.Fatal(err)
		}
		bound, err := m.BindAccount(s, 1, personal)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bound.Transition(s, 2, d.MembershipActive, checks(t, s)); err != nil {
			t.Fatal(err)
		}
	}
	ma, mb := member(t, a, d.MembershipActive), member(t, b, d.MembershipActive)
	if _, err := ma.Transition(a, 1, d.MembershipSuspended, checks(t, a)); err != nil {
		t.Fatal(err)
	}
	bs, _ := mb.Snapshot(b)
	if bs.State != d.MembershipActive {
		t.Fatal("another organization changed")
	}
	managed, err := d.ManagedAccount(a, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := managed.Reference(b); err != d.ErrScope {
		t.Fatal(err)
	}
	managedCredential, err := d.Credential(a, managed, d.EnrolledDeviceKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := managedCredential.Reference(b); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, _, err := personal.Reference(kernel.OrgScope{}); err != d.ErrScope {
		t.Fatal(err)
	}
	pending, err := d.NewInvited(a, memberID, employee(t, a), d.ManagedAccess)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pending.BindAccount(a, 1, personal); err != d.ErrInvalid {
		t.Fatal("mixed access models", err)
	}
	foreign, err := d.ManagedAccount(b, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pending.BindAccount(a, 1, foreign); err != d.ErrScope {
		t.Fatal(err)
	}
	bound, err := pending.BindAccount(a, 1, managed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.BindAccount(a, 2, managed); err != d.ErrConflict {
		t.Fatal(err)
	}
	if _, err := bound.Transition(a, 2, d.MembershipActive, checks(t, a)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []d.CredentialKind{d.AccountKey, d.EmailCode, d.LinkedProvider, d.EnrolledDeviceKey} {
		c, err := d.Credential(a, personal, kind)
		if err != nil {
			t.Fatal(err)
		}
		ref, k, err := c.Reference(a)
		if err != nil || ref != personal || k != kind {
			t.Fatal("credential metadata", err)
		}
	}
	if _, err := d.Credential(a, managed, d.AccountKey); err != d.ErrInvalid {
		t.Fatal("manager-known managed credential", err)
	}
	if _, err := d.Credential(a, managed, d.EnrolledDeviceKey); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Credential(a, foreign, d.EnrolledDeviceKey); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := d.Credential(a, personal, "password material"); err != d.ErrInvalid {
		t.Fatal(err)
	}
}

func TestLifecycleFactsFailClosed(t *testing.T) {
	s := scope(t, own)
	e := employee(t, s)
	m := member(t, s, d.MembershipInvited)
	for mask := 0; mask < 8; mask++ {
		f := checks(t, s)
		f.AdmissionConfirmed = mask&1 != 0
		f.ConfirmationSatisfied = mask&2 != 0
		f.LiveRoleAssignments = 0
		if mask&4 != 0 {
			f.LiveRoleAssignments = 1
		}
		_, err := m.Transition(s, 1, d.MembershipActive, f)
		if (err == nil) != (mask == 7) {
			t.Fatalf("activation facts %d: %v", mask, err)
		}
	}
	unbound, err := d.NewInvited(s, memberID, e, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unbound.Transition(s, 1, d.MembershipActive, checks(t, s)); err != d.ErrConditions {
		t.Fatal(err)
	}
	for _, state := range []d.EmployeeState{d.EmployeeInactive, d.EmployeeTerminated} {
		f := checks(t, s)
		f.Employee, err = d.RehydrateEmployee(s, d.EmployeeSnapshot{OrgID: s.OrgID(), ID: employeeID, State: state, Revision: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Transition(s, 1, d.MembershipActive, f); err != d.ErrConditions {
			t.Fatal(err)
		}
		if _, err := d.NewInvited(s, memberID, f.Employee, d.ExistingAccount); err != d.ErrConditions {
			t.Fatal(err)
		}
	}
	f := checks(t, s)
	f.LiveRoleAssignments = -1
	if _, err := m.Transition(s, 1, d.MembershipActive, f); err != d.ErrConditions {
		t.Fatal(err)
	}
	f = checks(t, s)
	f.Employee = d.Employee{}
	if _, err := m.Transition(s, 1, d.MembershipActive, f); err != d.ErrScope {
		t.Fatal(err)
	}
	f.Employee, err = d.NewEmployee(s, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Transition(s, 1, d.MembershipActive, f); err != d.ErrScope {
		t.Fatal(err)
	}
	for mask := 0; mask < 8; mask++ {
		ec := d.EmployeeConditions{OwnershipProtected: mask&1 != 0, ResponsibilitiesReviewed: mask&2 != 0, ReasonRecorded: mask&4 != 0}
		if _, err := e.Transition(s, 1, d.EmployeeInactive, ec); (err == nil) != (mask&5 == 5) {
			t.Fatalf("employee suspension facts %d: %v", mask, err)
		}
		if _, err := e.Transition(s, 1, d.EmployeeTerminated, ec); (err == nil) != (mask&3 == 3) {
			t.Fatalf("employee termination facts %d: %v", mask, err)
		}
	}
	active := member(t, s, d.MembershipActive)
	for _, mutate := range []func(*d.MembershipConditions){
		func(f *d.MembershipConditions) { f.OwnershipProtected = false }, func(f *d.MembershipConditions) { f.ReasonRecorded = false }, func(f *d.MembershipConditions) { f.Suspension = "unknown" },
	} {
		f := checks(t, s)
		mutate(&f)
		if _, err := active.Transition(s, 1, d.MembershipSuspended, f); err != d.ErrConditions {
			t.Fatal(err)
		}
	}
	for _, mutate := range []func(*d.MembershipConditions){
		func(f *d.MembershipConditions) { f.OwnershipProtected = false }, func(f *d.MembershipConditions) { f.ResponsibilitiesReviewed = false }, func(f *d.MembershipConditions) { f.EndReason = "unknown" }, func(f *d.MembershipConditions) { f.EndReason = d.AdmissionEnded },
	} {
		f := checks(t, s)
		mutate(&f)
		if _, err := active.Transition(s, 1, d.MembershipEnded, f); err != d.ErrConditions {
			t.Fatal(err)
		}
	}
	f = checks(t, s)
	f.EndReason = ""
	if _, err := m.Transition(s, 1, d.MembershipEnded, f); err != d.ErrConditions {
		t.Fatal(err)
	}
	for _, reason := range []d.EndReason{d.Offboarded, d.Left, d.AccountDeleted, d.OrganizationClosed} {
		f := checks(t, s)
		f.EndReason = reason
		if _, err := active.Transition(s, 1, d.MembershipEnded, f); err != nil {
			t.Fatal(err)
		}
	}
	f = checks(t, s)
	f.EndReason = d.AdmissionEnded
	if _, err := m.Transition(s, 1, d.MembershipEnded, f); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyInactivitySuspensionsResumeAutomatically(t *testing.T) {
	s := scope(t, own)
	active := member(t, s, d.MembershipActive)
	f := checks(t, s)
	f.Suspension = d.ByEmployeeInactivity
	if _, err := active.Transition(s, 1, d.MembershipSuspended, f); err != d.ErrConditions {
		t.Fatal("active employee accepted as inactive", err)
	}
	inactive, err := employee(t, s).Transition(s, 1, d.EmployeeInactive, d.EmployeeConditions{OwnershipProtected: true, ReasonRecorded: true})
	if err != nil {
		t.Fatal(err)
	}
	f.Employee = inactive
	suspended, err := active.Transition(s, 1, d.MembershipSuspended, f)
	if err != nil {
		t.Fatal(err)
	}
	restored := checks(t, s)
	restored.EmployeeStateChange = true
	if _, err := suspended.Transition(s, 2, d.MembershipActive, restored); err != nil {
		t.Fatal(err)
	}
	managerSuspended := member(t, s, d.MembershipSuspended)
	if _, err := managerSuspended.Transition(s, 1, d.MembershipActive, restored); err != d.ErrConditions {
		t.Fatal("automatic return lifted managerial suspension", err)
	}
	restored.EmployeeStateChange = false
	if _, err := managerSuspended.Transition(s, 1, d.MembershipActive, restored); err != nil {
		t.Fatal(err)
	}
	f.Employee = d.Employee{}
	if _, err := active.Transition(s, 1, d.MembershipSuspended, f); err != d.ErrScope {
		t.Fatal(err)
	}
	f.Employee, err = d.NewEmployee(s, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := active.Transition(s, 1, d.MembershipSuspended, f); err != d.ErrScope {
		t.Fatal(err)
	}
}

func TestOneLiveMembershipWithinKnownOrganizationSet(t *testing.T) {
	s := scope(t, own)
	active := member(t, s, d.MembershipActive)
	ended := member(t, s, d.MembershipEnded)
	if err := d.ValidateLiveMemberships(s, []d.Membership{ended, active}); err != nil {
		t.Fatal(err)
	}
	if err := d.ValidateLiveMemberships(s, []d.Membership{active, member(t, s, d.MembershipSuspended)}); err != d.ErrConflict {
		t.Fatal("duplicate employee accepted", err)
	}
	snap, _ := active.Snapshot(s)
	snap.EmployeeID = other
	snap.ID = enrollmentID
	second, err := d.RehydrateMembership(s, snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ValidateLiveMemberships(s, []d.Membership{active, second}); err != d.ErrConflict {
		t.Fatal("duplicate account accepted", err)
	}
	anotherEmployee, err := d.NewEmployee(s, other)
	if err != nil {
		t.Fatal(err)
	}
	firstPending, err := d.NewInvited(s, memberID, employee(t, s), d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	secondPending, err := d.NewInvited(s, enrollmentID, anotherEmployee, d.ManagedAccess)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ValidateLiveMemberships(s, []d.Membership{firstPending, secondPending}); err != nil {
		t.Fatal("unknown accounts conflated", err)
	}
	if err := d.ValidateLiveMemberships(s, []d.Membership{firstPending, firstPending}); err != d.ErrConflict {
		t.Fatal("invited employee duplicated", err)
	}
	if err := d.ValidateLiveMemberships(s, []d.Membership{active, member(t, scope(t, other), d.MembershipActive)}); err != d.ErrScope {
		t.Fatal("foreign set accepted", err)
	}
	if err := d.ValidateLiveMemberships(kernel.OrgScope{}, nil); err != d.ErrScope {
		t.Fatal(err)
	}
	if err := d.ValidateLiveMemberships(s, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceProofLimitsAndRevocationClasses(t *testing.T) {
	s := scope(t, own)
	m := member(t, s, d.MembershipActive)
	fp := "fp:" + strings.Repeat("a", 64)
	for limit := int64(1); limit <= 5; limit++ {
		facts := d.EnrollmentConditions{ProofConfirmed: true, EnrolledDevices: limit - 1, DeviceLimit: limit}
		device, err := d.EnrollDevice(s, enrollmentID, deviceID, fp, m, facts)
		if err != nil {
			t.Fatal(err)
		}
		snap, _ := device.Snapshot(s)
		if snap.MembershipID != memberID || snap.DeviceID != deviceID || snap.Fingerprint != fp || snap.State != d.DeviceEnrolled {
			t.Fatal("enrolment association")
		}
		facts.EnrolledDevices++
		if _, err := d.EnrollDevice(s, enrollmentID, deviceID, fp, m, facts); err != d.ErrConditions {
			t.Fatal(err)
		}
		for _, class := range []d.RevocationClass{d.LostOrCompromised, d.AccessEnded} {
			revoked, err := device.Transition(s, 1, d.DeviceRevoked, class)
			if err != nil {
				t.Fatal(err)
			}
			snap, _ := revoked.Snapshot(s)
			if snap.Revocation != class {
				t.Fatal("revocation class lost")
			}
			if _, err := revoked.Transition(s, 2, d.DeviceEnrolled, class); err != d.ErrRevoked {
				t.Fatal(err)
			}
		}
		if _, err := device.Transition(s, 1, d.DeviceRevoked, ""); err != d.ErrConditions {
			t.Fatal(err)
		}
	}
	for _, facts := range []d.EnrollmentConditions{{}, {ProofConfirmed: true, DeviceLimit: 0}, {ProofConfirmed: true, DeviceLimit: 6}, {ProofConfirmed: true, DeviceLimit: 3, EnrolledDevices: -1}, {DeviceLimit: 3}} {
		if _, err := d.EnrollDevice(s, enrollmentID, deviceID, fp, m, facts); err != d.ErrConditions {
			t.Fatal(err)
		}
	}
	facts := d.EnrollmentConditions{ProofConfirmed: true, DeviceLimit: 3}
	if _, err := d.EnrollDevice(s, enrollmentID, deviceID, fp, member(t, s, d.MembershipSuspended), facts); err != d.ErrConditions {
		t.Fatal(err)
	}
	if _, err := d.EnrollDevice(s, enrollmentID, deviceID, fp, d.Membership{}, facts); err != d.ErrScope {
		t.Fatal(err)
	}
	managed, err := d.ManagedAccount(s, accountID)
	if err != nil {
		t.Fatal(err)
	}
	ms, _ := m.Snapshot(s)
	ms.Model = d.ManagedAccess
	ms.Account = managed
	mm, err := d.RehydrateMembership(s, ms)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.EnrollDevice(s, enrollmentID, deviceID, fp, mm, facts); err != d.ErrConditions {
		t.Fatal("managed device ceiling", err)
	}
	facts.DeviceLimit = 1
	if _, err := d.EnrollDevice(s, enrollmentID, deviceID, fp, mm, facts); err != nil {
		t.Fatal(err)
	}
}

func TestScopesRevisionsAndZeroValues(t *testing.T) {
	s, foreign := scope(t, own), scope(t, other)
	e, m := employee(t, s), member(t, s, d.MembershipInvited)
	device, err := d.EnrollDevice(s, enrollmentID, deviceID, "fp:"+strings.Repeat("a", 64), member(t, s, d.MembershipActive), d.EnrollmentConditions{ProofConfirmed: true, DeviceLimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := d.DriverProfile(s, e)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []kernel.OrgScope{{}, foreign} {
		if _, err := e.Snapshot(bad); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := m.Snapshot(bad); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := device.Snapshot(bad); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := profile.EmployeeID(bad); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := d.DriverProfile(bad, e); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := d.NewInvited(bad, memberID, e, d.ExistingAccount); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := m.BindAccount(bad, 1, account(t)); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := m.Transition(bad, 1, d.MembershipActive, checks(t, s)); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := e.Transition(bad, 1, d.EmployeeInactive, d.EmployeeConditions{}); err != d.ErrScope {
			t.Fatal(err)
		}
		if _, err := device.Transition(bad, 1, d.DeviceRevoked, d.AccessEnded); err != d.ErrScope {
			t.Fatal(err)
		}
	}
	for _, expected := range []int64{-1, 0, 2} {
		if _, err := e.Transition(s, expected, d.EmployeeInactive, d.EmployeeConditions{}); err != d.ErrConflict {
			t.Fatal(err)
		}
		if _, err := m.BindAccount(s, expected, account(t)); err != d.ErrConflict {
			t.Fatal(err)
		}
		if _, err := device.Transition(s, expected, d.DeviceRevoked, d.AccessEnded); err != d.ErrConflict {
			t.Fatal(err)
		}
	}
	es, _ := e.Snapshot(s)
	es.Revision = math.MaxInt64
	maxEmployee, err := d.RehydrateEmployee(s, es)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maxEmployee.Transition(s, math.MaxInt64, d.EmployeeInactive, d.EmployeeConditions{}); err != d.ErrRevision {
		t.Fatal(err)
	}
	ms, _ := m.Snapshot(s)
	ms.Revision = math.MaxInt64
	maxMember, err := d.RehydrateMembership(s, ms)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maxMember.BindAccount(s, math.MaxInt64, account(t)); err != d.ErrRevision {
		t.Fatal(err)
	}
	ds, _ := device.Snapshot(s)
	ds.Revision = math.MaxInt64
	maxDevice, err := d.RehydrateDevice(s, ds)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maxDevice.Transition(s, math.MaxInt64, d.DeviceRevoked, d.AccessEnded); err != d.ErrRevision {
		t.Fatal(err)
	}
	if _, err := (d.Employee{}).Snapshot(s); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := (d.Membership{}).Snapshot(s); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := (d.DeviceEnrollment{}).Snapshot(s); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := (d.DriverProfileRef{}).EmployeeID(s); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, _, err := (d.CredentialRef{}).Reference(s); err != d.ErrInvalid {
		t.Fatal(err)
	}
	if _, _, err := (d.AccountRef{}).Reference(s); err != d.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := member(t, s, d.MembershipActive).BindAccount(s, 1, account(t)); err != d.ErrTransition {
		t.Fatal(err)
	}
	es.MergedInto = other
	es.State = d.EmployeeTerminated
	es.Revision = 1
	merged, err := d.RehydrateEmployee(s, es)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := merged.Transition(s, 1, d.EmployeeActive, d.EmployeeConditions{}); err != d.ErrMerged {
		t.Fatal(err)
	}
}

func TestIdentityHydrationRejectsInvalidShapes(t *testing.T) {
	s := scope(t, own)
	for _, raw := range []string{"", "00000000-0000-0000-0000-000000000000", "not-an-id", accountID + " ", "7b0e2a54_0000-4000-8000-000000000205"} {
		if _, err := d.PersonalAccount(raw); err != d.ErrInvalid {
			t.Fatal(err)
		}
		if _, err := d.ManagedAccount(s, raw); err != d.ErrInvalid {
			t.Fatal(err)
		}
	}
	upper, err := d.PersonalAccount(strings.ToUpper(accountID))
	if err != nil {
		t.Fatal(err)
	}
	if id, kind, err := upper.Reference(s); id != accountID || kind != d.ExistingAccount || err != nil {
		t.Fatal("UUID canonicalization", err)
	}
	if _, err := d.ManagedAccount(kernel.OrgScope{}, accountID); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := d.Credential(s, d.AccountRef{}, d.AccountKey); err != d.ErrInvalid {
		t.Fatal(err)
	}
	unbound, err := d.NewInvited(s, memberID, employee(t, s), d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unbound.BindAccount(s, 1, d.AccountRef{}); err != d.ErrInvalid {
		t.Fatal(err)
	}
	base := d.EmployeeSnapshot{OrgID: s.OrgID(), ID: employeeID, Revision: 1, State: d.EmployeeActive}
	for _, mutate := range []func(*d.EmployeeSnapshot){
		func(v *d.EmployeeSnapshot) { v.ID = "" }, func(v *d.EmployeeSnapshot) { v.Revision = 0 }, func(v *d.EmployeeSnapshot) { v.State = "unknown" }, func(v *d.EmployeeSnapshot) { v.MergedInto = "bad" }, func(v *d.EmployeeSnapshot) { v.State = d.EmployeeTerminated; v.MergedInto = employeeID }, func(v *d.EmployeeSnapshot) { v.MergedInto = other },
	} {
		v := base
		mutate(&v)
		if _, err := d.RehydrateEmployee(s, v); err != d.ErrInvalid {
			t.Fatal(err)
		}
	}
	if _, err := d.RehydrateEmployee(scope(t, other), base); err != d.ErrScope {
		t.Fatal(err)
	}
	mb, _ := member(t, s, d.MembershipActive).Snapshot(s)
	for _, mutate := range []func(*d.MembershipSnapshot){
		func(v *d.MembershipSnapshot) { v.ID = "" }, func(v *d.MembershipSnapshot) { v.EmployeeID = "" }, func(v *d.MembershipSnapshot) { v.Revision = 0 }, func(v *d.MembershipSnapshot) { v.Model = "unknown" }, func(v *d.MembershipSnapshot) { v.Model = d.ManagedAccess }, func(v *d.MembershipSnapshot) { v.State = "unknown" }, func(v *d.MembershipSnapshot) { v.Account = d.AccountRef{} }, func(v *d.MembershipSnapshot) { v.State = d.MembershipInvited; v.EndReason = d.Left }, func(v *d.MembershipSnapshot) { v.State = d.MembershipSuspended }, func(v *d.MembershipSnapshot) { v.State = d.MembershipEnded }, func(v *d.MembershipSnapshot) {
			v.State = d.MembershipEnded
			v.EndReason = d.Left
			v.Suspension = "unknown"
		},
	} {
		v := mb
		mutate(&v)
		if _, err := d.RehydrateMembership(s, v); err != d.ErrInvalid {
			t.Fatal(err)
		}
	}
	foreign, err := d.ManagedAccount(scope(t, other), accountID)
	if err != nil {
		t.Fatal(err)
	}
	ms := mb
	ms.Model = d.ManagedAccess
	ms.Account = foreign
	if _, err := d.RehydrateMembership(s, ms); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := d.RehydrateMembership(scope(t, other), mb); err != d.ErrScope {
		t.Fatal(err)
	}
	db := d.DeviceSnapshot{OrgID: s.OrgID(), ID: enrollmentID, MembershipID: memberID, DeviceID: deviceID, Fingerprint: "fp:" + strings.Repeat("a", 64), State: d.DeviceEnrolled, Revision: 1}
	for _, mutate := range []func(*d.DeviceSnapshot){
		func(v *d.DeviceSnapshot) { v.ID = "" }, func(v *d.DeviceSnapshot) { v.MembershipID = "" }, func(v *d.DeviceSnapshot) { v.DeviceID = "" }, func(v *d.DeviceSnapshot) { v.Fingerprint = "bad" }, func(v *d.DeviceSnapshot) { v.Fingerprint = " " + v.Fingerprint }, func(v *d.DeviceSnapshot) { v.Revision = 0 }, func(v *d.DeviceSnapshot) { v.State = "unknown" }, func(v *d.DeviceSnapshot) { v.Revocation = d.AccessEnded }, func(v *d.DeviceSnapshot) { v.State = d.DeviceRevoked },
	} {
		v := db
		mutate(&v)
		if _, err := d.RehydrateDevice(s, v); err != d.ErrInvalid {
			t.Fatal(err)
		}
	}
	if _, err := d.RehydrateDevice(scope(t, other), db); err != d.ErrScope {
		t.Fatal(err)
	}
}

func TestImmutableValuesAreSafeToShareAndRedacted(t *testing.T) {
	s := scope(t, own)
	e, m := employee(t, s), member(t, s, d.MembershipActive)
	before, _ := e.Snapshot(s)
	membershipBefore, _ := m.Snapshot(s)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			after, err := e.Transition(s, 1, d.EmployeeInactive, d.EmployeeConditions{OwnershipProtected: true, ReasonRecorded: true})
			if err != nil {
				t.Error(err)
			}
			snap, _ := after.Snapshot(s)
			snap.ID = other
			if _, err := m.Transition(s, 1, d.MembershipSuspended, checks(t, s)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	unchanged, _ := e.Snapshot(s)
	unchangedMember, _ := m.Snapshot(s)
	if !reflect.DeepEqual(before, unchanged) || !reflect.DeepEqual(membershipBefore, unchangedMember) {
		t.Fatal("shared values mutated")
	}
	profile, err := d.DriverProfile(s, e)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := d.Credential(s, account(t), d.AccountKey)
	if err != nil {
		t.Fatal(err)
	}
	device, err := d.EnrollDevice(s, enrollmentID, deviceID, "fp:"+strings.Repeat("a", 64), m, d.EnrollmentConditions{ProofConfirmed: true, DeviceLimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	ds, _ := device.Snapshot(s)
	values := []any{e, before, m, membershipBefore, account(t), credential, profile, device, ds}
	for _, v := range values {
		var output bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		logger.Info("synthetic identity", "value", v, "nested", []any{v})
		raw, err := json.Marshal(map[string]any{"values": []any{v}})
		if err != nil {
			t.Fatal(err)
		}
		text := fmt.Sprintf("%v %+v %#v %d %s %q", v, v, v, v, v, v) + string(raw) + output.String()
		if !strings.Contains(text, "redacted") {
			t.Fatal("missing redaction")
		}
		for _, secret := range []string{own, employeeID, memberID, accountID, deviceID, enrollmentID, "fp:"} {
			if strings.Contains(text, secret) {
				t.Fatalf("identity leaked through formatting: %T", v)
			}
		}
	}
}
