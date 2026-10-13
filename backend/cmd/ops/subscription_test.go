package main

import (
	"strings"
	"testing"
)

func TestSubscriptionOperationsRefuseAnonymousOrUnboundedInput(t *testing.T) {
	for _, args := range [][]string{{"stats"}, {"replay", "--job", "bad", "--reason", "provider-fixed", "--operator", "synthetic"}, {"replay", "--job", "11111111-1111-4111-8111-111111111111", "--reason", "raw-purchase-token", "--operator", "synthetic"}, {"stats", "--purchase-token", "synthetic-secret"}} {
		err := runSubscription(args, getenvNone)
		if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("unsafe operator args")
		}
	}
}
