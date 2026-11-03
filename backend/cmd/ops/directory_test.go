package main

import (
	"strings"
	"testing"
)

func TestDirectoryFactReprojectionRequiresAccountableExplicitRequest(t *testing.T) {
	for _, args := range [][]string{{"directory"}, {"directory", "reproject-facts", "--campaign", "11111111-1111-4111-8111-111111111111"}, {"directory", "facts-status"}} {
		if err := run(args, func(string) string { return "" }); err == nil {
			t.Fatal("incomplete request accepted", args)
		}
	}
	err := run([]string{"directory", "reproject-facts", "--campaign", "11111111-1111-4111-8111-111111111111", "--reason", "synthetic"}, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "operator identity required") {
		t.Fatal("anonymous request reached I/O", err)
	}
}
