package domain_test

import (
	"errors"
	"reflect"
	"testing"

	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestSetupChecklistReusesActivationFactsWithoutChangingState(t *testing.T) {
	organization, own := pending(t)
	facts := org.Conditions{ActiveUnits: 1, ActiveOwners: 1, TermsRecorded: true}
	missing, err := organization.SetupChecklist(own, facts)
	if err != nil || len(missing) != 0 {
		t.Fatal("complete persisted setup rejected")
	}
	incomplete, err := org.NewPending(own, org.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	missing, err = incomplete.SetupChecklist(own, org.Conditions{})
	want := []org.Missing{org.MissingName, org.MissingTimeZone, org.MissingUnit, org.MissingOwner, org.MissingTerms}
	if err != nil || !reflect.DeepEqual(missing, want) {
		t.Fatal("checklist diverges from activation requirements")
	}
	missing[0] = "changed"
	missing, err = incomplete.SetupChecklist(own, org.Conditions{})
	if err != nil || !reflect.DeepEqual(missing, want) {
		t.Fatal("checklist aliases caller data")
	}
	if snapshot(t, incomplete, own).State != org.PendingSetup || snapshot(t, incomplete, own).Revision != 1 {
		t.Fatal("read checklist changed lifecycle authority")
	}
	if _, err := incomplete.SetupChecklist(scope(t, foreignID), facts); !errors.Is(err, org.ErrScope) {
		t.Fatal("foreign checklist scope accepted")
	}
	if _, err := incomplete.SetupChecklist(own, org.Conditions{ActiveUnits: -1}); !errors.Is(err, org.ErrInvalid) {
		t.Fatal("untrusted negative facts accepted")
	}
}
