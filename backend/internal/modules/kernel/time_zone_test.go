package kernel_test

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestLoadTimeZoneSharedProfile(t *testing.T) {
	tests := []struct {
		id string
		ok bool
	}{
		{"UTC", true},
		{"America/Cuiaba", true},
		{"America/Argentina/Buenos_Aires", true},
		{"Etc/GMT+3", true},
		{"US/Eastern", true},
		{"", false},
		{"Local", false},
		{"GMT", false},
		{"EST", false},
		{"Japan", false},
		{"Factory", false},
		{"localtime", false},
		{"posixrules", false},
		{"utc", false},
		{"posix/America/Cuiaba", false},
		{"right/America/Cuiaba", false},
		{"SystemV/EST5", false},
		{"SystemV/AST4", false},
		{"America//Cuiaba", false},
		{"/America/Cuiaba", false},
		{"America/Cuiaba/", false},
		{"../etc/passwd", false},
		{" America/Cuiaba", false},
		{"Mars/Olympus", false},
	}
	for _, tt := range append(tests, tests...) { // second pass reads the cache
		loc, ok := kernel.LoadTimeZone(tt.id)
		if ok != tt.ok || (ok && loc.String() != tt.id) || (!ok && loc != nil) {
			t.Errorf("LoadTimeZone(%q) = %v, %v; want ok=%v", tt.id, loc, ok, tt.ok)
		}
	}
}
