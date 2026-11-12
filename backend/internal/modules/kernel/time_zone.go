package kernel

import (
	"regexp"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Standard-library fallback when the host has no IANA database.
)

var (
	timeZonePattern = regexp.MustCompile(`^[A-Za-z0-9_+/-]{1,128}$`)
	// time.LoadLocation reads the zone file on every call; accepted names are a
	// small fixed set, so only successful loads are kept for the process.
	loadedZones sync.Map
)

// LoadTimeZone resolves the one zone profile shared by Abastevo Empresas
// organizations, units and operation times (BR-TIME-004): UTC or an IANA
// Area/Location name. Abbreviations and single-segment names (GMT, EST,
// Japan, Local) and host-only entries (localtime, posixrules, Factory, the
// posix/, right/ and SystemV/ trees) are refused, so a stored zone resolves
// the same way on every server and in the Android client.
func LoadTimeZone(id string) (*time.Location, bool) {
	if !timeZonePattern.MatchString(id) || (id != "UTC" && !strings.Contains(id, "/")) ||
		strings.Contains(id, "//") || strings.HasPrefix(id, "/") || strings.HasSuffix(id, "/") {
		return nil, false
	}
	for _, tree := range []string{"posix/", "right/", "SystemV/"} {
		if strings.HasPrefix(id, tree) {
			return nil, false
		}
	}
	if loc, ok := loadedZones.Load(id); ok {
		return loc.(*time.Location), true
	}
	loc, err := time.LoadLocation(id)
	if err != nil {
		return nil, false
	}
	loadedZones.Store(id, loc)
	return loc, true
}
