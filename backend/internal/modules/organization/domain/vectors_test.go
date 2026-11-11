package domain_test

import (
	"bufio"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func scope(t *testing.T, raw string) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(raw)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
func fields(t *testing.T, raw string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, field := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" {
			t.Fatal("bad vector field")
		}
		if _, found := out[key]; found {
			t.Fatal("duplicate vector field")
		}
		out[key] = value
	}
	return out
}
func integer(t *testing.T, raw string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func boolean(t *testing.T, raw string) bool {
	t.Helper()
	b, err := strconv.ParseBool(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func tags(raw string) []org.Kind {
	if raw == "" {
		return nil
	}
	var result []org.Kind
	for _, kind := range strings.Split(raw, ",") {
		result = append(result, org.Kind(kind))
	}
	return result
}
func errorCode(t *testing.T, err error) map[string]string {
	t.Helper()
	switch {
	case errors.Is(err, org.ErrSetup):
		var setup *org.SetupError
		if !errors.As(err, &setup) {
			t.Fatal("missing checklist")
		}
		var names []string
		for _, item := range setup.Missing() {
			names = append(names, string(item))
		}
		return map[string]string{"error": "setup", "missing": strings.Join(names, ",")}
	case errors.Is(err, org.ErrInvalid):
		return map[string]string{"error": "invalid"}
	case errors.Is(err, org.ErrClosed):
		return map[string]string{"error": "closed"}
	case errors.Is(err, org.ErrTransition):
		return map[string]string{"error": "transition"}
	case errors.Is(err, org.ErrConditions):
		return map[string]string{"error": "conditions"}
	case errors.Is(err, org.ErrConflict):
		return map[string]string{"error": "conflict"}
	case errors.Is(err, org.ErrRevision):
		return map[string]string{"error": "revision"}
	default:
		t.Fatal("unexpected domain error", err)
	}
	return nil
}
func execute(t *testing.T, a map[string]string) map[string]string {
	t.Helper()
	s := scope(t, "7b0e2a54-0000-4000-8000-000000000001")
	organization, err := org.Rehydrate(s, org.Snapshot{
		ID: s.OrgID(), Profile: org.Profile{Name: a["name"], TimeZone: a["zone"], Kinds: tags(a["kinds"])},
		State: org.State(a["from"]), Revision: integer(t, a["revision"]), SuspensionReason: org.Reason(a["prior_reason"]),
	})
	if err != nil {
		return errorCode(t, err)
	}
	changed, err := organization.Transition(s, integer(t, a["expected_revision"]), org.State(a["to"]), org.Conditions{
		ActiveUnits: integer(t, a["units"]), ActiveOwners: integer(t, a["owners"]), TermsRecorded: boolean(t, a["terms"]),
		NoOperationalRecords: boolean(t, a["no_records"]), SuspensionReason: org.Reason(a["reason"]),
		SuspensionResolved: boolean(t, a["resolved"]), ArchiveClear: boolean(t, a["archive_clear"]), Closure: org.ClosureDisposition(a["closure"]),
	})
	if err != nil {
		return errorCode(t, err)
	}
	snapshot, err := changed.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, kind := range snapshot.Profile.Kinds {
		kinds = append(kinds, string(kind))
	}
	return map[string]string{"state": string(snapshot.State), "revision": strconv.FormatInt(snapshot.Revision, 10),
		"name": snapshot.Profile.Name, "zone": snapshot.Profile.TimeZone, "kinds": strings.Join(kinds, ","), "reason": string(snapshot.SuspensionReason)}
}
func TestOrganizationLifecycleVectors(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/enterprise/organization-lifecycle-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("bad vector header")
	}
	seen := map[string]bool{}
	for scanner.Scan() {
		row := strings.Split(scanner.Text(), "|")
		if len(row) != 4 || row[0] == "" || seen[row[0]] || row[1] != "TRANSITION" {
			t.Fatal("invalid/duplicate/unknown vector")
		}
		seen[row[0]] = true
		t.Run(row[0], func(t *testing.T) {
			got := execute(t, fields(t, row[2]))
			want := fields(t, row[3])
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v; want %v", got, want)
			}
		})
	}
	if scanner.Err() != nil || len(seen) != 111 {
		t.Fatalf("rows=%d error=%v", len(seen), scanner.Err())
	}
}
