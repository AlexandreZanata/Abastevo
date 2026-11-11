package domain_test

import (
	"encoding/json"
	"fmt"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"strings"
	"testing"
)

func TestAccountTombstoneCannotRestoreLivePersonalLink(t *testing.T) {
	s := scope(t, own)
	before, _ := member(t, s, d.MembershipEnded).Snapshot(s)
	snapshot := before
	snapshot.Account = d.AccountRef{}
	snapshot.AccountTombstone = strings.ToUpper(other)
	value, err := d.RehydrateMembership(s, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	got, err := value.Snapshot(s)
	if err != nil || got.AccountTombstone != other || got.Account != (d.AccountRef{}) {
		t.Fatal("tombstone hydration", err)
	}
	if _, err := value.BindAccount(s, 1, account(t)); err != d.ErrEnded {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{"snapshot": got})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded)+fmt.Sprintf("%d", got), other) {
		t.Fatal("tombstone leaked")
	}
	for _, mutate := range []func(*d.MembershipSnapshot){
		func(v *d.MembershipSnapshot) { v.Account = account(t) },
		func(v *d.MembershipSnapshot) { v.AccountTombstone = "bad" },
		func(v *d.MembershipSnapshot) { v.State = d.MembershipInvited; v.EndReason = "" },
	} {
		v := snapshot
		mutate(&v)
		if _, err := d.RehydrateMembership(s, v); err != d.ErrInvalid {
			t.Fatal(err)
		}
	}
}
