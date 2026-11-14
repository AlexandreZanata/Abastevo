package jobs

import (
	"testing"

	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	forumapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
)

func TestStateMatchesTheEditorialForumFingerprint(t *testing.T) {
	fingerprint := directoryapp.ExportFingerprint{Stations: 2, Hash: "md5:a:b"}
	pins := []RevisionState{{RevisionID: "r1", SourceChecksum: "sha256:r1"}}
	state := State{
		DirectoryFingerprint: fingerprint.Hash, DirectoryStations: fingerprint.Stations,
		FromSurvey: "2026-01-01", ToSurvey: "2026-12-31", OfficialRevisions: pins,
	}
	forum := forumState(forumapp.EditorialExportFingerprint{Posts: 2, Hash: "md5:f"})
	if !stateMatches(state, fingerprint, pins, "2026-01-01", "2026-12-31", nil, nil) {
		t.Fatal("state without forum rejected")
	}
	if stateMatches(state, fingerprint, pins, "2026-01-01", "2026-12-31", nil, forum) {
		t.Error("a newly wired forum did not invalidate a forum-less state")
	}
	withForum := state
	withForum.Forum = forum
	if !stateMatches(withForum, fingerprint, pins, "2026-01-01", "2026-12-31", nil, forumState(forumapp.EditorialExportFingerprint{Posts: 2, Hash: "md5:f"})) {
		t.Error("matching forum fingerprint rejected")
	}
	for name, changed := range map[string]*ForumState{
		"edit":          forumState(forumapp.EditorialExportFingerprint{Posts: 2, Hash: "md5:g"}),
		"hide":          forumState(forumapp.EditorialExportFingerprint{Posts: 1, Hash: "md5:h"}),
		"switch off":    nil,
		"publish first": forumState(forumapp.EditorialExportFingerprint{Posts: 3, Hash: "md5:f"}),
	} {
		if stateMatches(withForum, fingerprint, pins, "2026-01-01", "2026-12-31", nil, changed) {
			t.Errorf("forum %s accepted as unchanged", name)
		}
	}
}

func TestExportStateKeyIncludesTheForumFingerprint(t *testing.T) {
	fingerprint := directoryapp.ExportFingerprint{Stations: 2, Hash: "md5:a:b"}
	pins := []RevisionState{{RevisionID: "r1", SourceChecksum: "sha256:r1"}}
	base := exportStateKey(fingerprint, pins, nil, nil)
	first := exportStateKey(fingerprint, pins, nil, forumState(forumapp.EditorialExportFingerprint{Posts: 1, Hash: "md5:f"}))
	second := exportStateKey(fingerprint, pins, nil, forumState(forumapp.EditorialExportFingerprint{Posts: 1, Hash: "md5:g"}))
	if base == first || first == second {
		t.Errorf("forum fingerprint did not change the reconcile dedupe key: %s %s %s", base, first, second)
	}
}
