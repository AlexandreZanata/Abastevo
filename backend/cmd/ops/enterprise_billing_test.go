package main

import (
	"io"
	"strings"
	"testing"
)

func TestReviewedChargeCommandRefusesProviderReferences(t *testing.T) {
	err := runEnterprise([]string{"billing", "bind", "--resource-ref", "synthetic-invoice"},
		func(string) string { return "" }, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "ops enterprise billing bind --org") {
		t.Fatalf("expected binding usage and no provider input, got %v", err)
	}
}
