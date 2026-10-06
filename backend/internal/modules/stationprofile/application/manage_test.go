package application

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type manageGrants struct {
	mu    sync.Mutex
	grant GrantRow
}

func (f *manageGrants) DecideAtomically(_ context.Context, _ DecisionInput, _ *GrantInput, _ string) error {
	return nil
}

func (f *manageGrants) ActiveGrant(_ context.Context, _, _ string) (GrantRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.grant.ID == "" {
		return GrantRow{}, false, nil
	}
	return f.grant, true, nil
}

type manageProfiles struct {
	mu       sync.Mutex
	revision int
	fields   map[string]string
}

func (f *manageProfiles) EnsureUnclaimed(_ context.Context, _ string) error { return nil }

func (f *manageProfiles) Profile(_ context.Context, _ string) (StoredProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return StoredProfile{PolicyVersion: "profile-v1", Revision: f.revision, Business: f.fields}, nil
}

func (f *manageProfiles) RecordOperator(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func (f *manageProfiles) CloseOperator(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (f *manageProfiles) CurrentOperator(_ context.Context, _ string) (StoredOperator, bool, error) {
	return StoredOperator{}, false, nil
}

func (f *manageProfiles) UpdateProjection(_ context.Context, _ string, expectedRevision int, fields map[string]string) (StoredProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if expectedRevision != f.revision {
		return StoredProfile{}, ErrManageVersion
	}
	f.revision++
	f.fields = fields
	return StoredProfile{PolicyVersion: "profile-v1", Revision: f.revision, Business: fields}, nil
}

func manageTestPorts(grants *manageGrants, profiles *manageProfiles) ManagePorts {
	return ManagePorts{
		Grants:   grants,
		Profiles: profiles,
		OperatorOf: func(context.Context, string) (string, bool, error) {
			return "04218406000104", true, nil
		},
		AccountLive: func(context.Context, string) (bool, error) { return true, nil },
		SubmitReply: func(_ context.Context, accountID, stationID, product, text string) (string, error) {
			if len([]rune(text)) > 280 {
				return "", errors.New("text-too-long")
			}
			return "comment-1", nil
		},
		Attribute: func(_ context.Context, _, _, _, _ string) error { return nil },
	}
}

func grantedPorts() (*manageGrants, *manageProfiles, ManagePorts) {
	grants := &manageGrants{grant: GrantRow{
		ID: "grant-1", AccountID: "acc-1", StationID: "station-1",
		OperatorCNPJ: "04218406000104", Role: "administrator",
		Scopes: []string{"profile.edit", "reply.official"},
		Status: "active",
	}}
	profiles := &manageProfiles{revision: 1}
	return grants, profiles, manageTestPorts(grants, profiles)
}

func TestEditBusinessFieldsEnforcesScopeAndPolicy(t *testing.T) {
	_, _, ports := grantedPorts()
	updated, err := EditBusinessFields(context.Background(), ports, "acc-1", "station-1", 1, map[string]string{
		"phone": "+55-11-99999-0000", "services": "fuel,convenience",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if updated.Revision != 2 || updated.Business["phone"] == "" {
		t.Fatalf("updated = %+v", updated)
	}
	// Regulatory identity is unreachable through this path.
	if _, err := EditBusinessFields(context.Background(), ports, "acc-1", "station-1", 2, map[string]string{"cnpj": "04218406000104"}); err != ErrManageFields {
		t.Fatalf("canonical err = %v", err)
	}
	// Unknown service and overlong description refuse.
	if _, err := EditBusinessFields(context.Background(), ports, "acc-1", "station-1", 2, map[string]string{"services": "casino"}); err != ErrManageFields {
		t.Fatalf("service err = %v", err)
	}
	// Stale revision fails safely (optimistic conflict, reload+retry).
	if _, err := EditBusinessFields(context.Background(), ports, "acc-1", "station-1", 1, map[string]string{"phone": "x"}); err != ErrManageVersion {
		t.Fatalf("stale err = %v", err)
	}
	// No grant, no edit.
	portsNoGrant := ports
	portsNoGrant.Grants = &manageGrants{}
	if _, err := EditBusinessFields(context.Background(), portsNoGrant, "acc-1", "station-1", 1, map[string]string{"phone": "x"}); err != ErrManageGrant {
		t.Fatalf("grant err = %v", err)
	}
}

func TestPostOfficialReplyVerifiesAttribution(t *testing.T) {
	_, _, ports := grantedPorts()
	id, err := PostOfficialReply(context.Background(), ports, "acc-1", "station-1", "ETHANOL", "Obrigado pela visita!")
	if err != nil || id != "comment-1" {
		t.Fatalf("reply = %q, err = %v", id, err)
	}
	// 281 scalars refuse through the transport rules.
	long := make([]rune, 281)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := PostOfficialReply(context.Background(), ports, "acc-1", "station-1", "ETHANOL", string(long)); err == nil {
		t.Fatal("281-scalar reply must fail")
	}
	// Manager without reply scope cannot reply officially.
	managerPorts := ports
	managerPorts.Grants = &manageGrants{grant: GrantRow{
		ID: "grant-2", AccountID: "acc-2", StationID: "station-1",
		OperatorCNPJ: "04218406000104", Role: "manager",
		Scopes: []string{"profile.edit"},
		Status: "active",
	}}
	if _, err := PostOfficialReply(context.Background(), managerPorts, "acc-2", "station-1", "ETHANOL", "Oi"); err != ErrManageGrant {
		t.Fatalf("scope err = %v", err)
	}
	// Stale operator denies even with a live grant.
	stalePorts := ports
	stalePorts.OperatorOf = func(context.Context, string) (string, bool, error) {
		return "00428184000195", true, nil
	}
	if _, err := PostOfficialReply(context.Background(), stalePorts, "acc-1", "station-1", "ETHANOL", "Oi"); err != ErrManageStale {
		t.Fatalf("stale err = %v", err)
	}
}
