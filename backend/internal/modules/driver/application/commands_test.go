package application

import (
	"context"
	"errors"
	"testing"
)

type stubAuthority struct {
	principal Principal
	err       error
}

func (s stubAuthority) Verify(context.Context, Proof) (Principal, error) { return s.principal, s.err }

func TestCommandServiceRejectsUnsignedOwnerOriginAndMismatch(t *testing.T) {
	ctx := context.Background()
	owner := "11111111-1111-4111-8111-111111111111"
	proof := Proof{Signature: []byte("synthetic"), RequestHash: "hash"}
	valid := Principal{Owner: owner, Origin: "one.example.test", Contributor: "22222222-2222-4222-8222-222222222222", KeyID: "33333333-3333-4333-8333-333333333333"}
	cases := []struct {
		name    string
		service *CommandService
	}{
		{"nil authority", &CommandService{Origin: "one.example.test"}},
		{"authority failure", &CommandService{Authority: stubAuthority{err: errors.New("bad signature")}, Origin: "one.example.test"}},
		{"owner mismatch", &CommandService{Authority: stubAuthority{principal: valid}, Origin: "one.example.test"}},
		{"origin mismatch", &CommandService{Authority: stubAuthority{principal: valid}, Origin: "other.example.test"}},
		{"missing contributor", &CommandService{Authority: stubAuthority{principal: Principal{Owner: owner, Origin: "one.example.test", KeyID: valid.KeyID}}, Origin: "one.example.test"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := owner
			if tc.name == "owner mismatch" {
				got = "44444444-4444-4444-8444-444444444444"
			}
			if _, err := tc.service.GetEarning(ctx, proof, got, "55555555-5555-4555-8555-555555555555"); !errors.Is(err, ErrSignedProof) {
				t.Fatal(err)
			}
		})
	}
}
