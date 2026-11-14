package enterpriseexport

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	forumapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	forumdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

type fakeForum struct {
	rows  []forumapp.EditorialExportPost
	stuck bool
}

func (f *fakeForum) ExportEditorialPage(_ context.Context, afterID string, limit int) ([]forumapp.EditorialExportPost, string, error) {
	if f.stuck {
		return nil, "00000000-0000-4000-8000-0000000000f1", nil
	}
	start := 0
	if afterID != "" {
		start = len(f.rows)
		for i, row := range f.rows {
			if row.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(f.rows))
	next := ""
	if end < len(f.rows) && end > start {
		next = f.rows[end-1].ID
	}
	return f.rows[start:end], next, nil
}

func forumFixture() *fakeForum {
	cuiaba := 5103403
	gasoline := "GASOLINE_REGULAR"
	station := "00000000-0000-4000-8000-000000000001"
	published := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	return &fakeForum{rows: []forumapp.EditorialExportPost{
		{
			ID: "00000000-0000-4000-8000-0000000000f1", Category: forumdomain.CategoryFuels,
			CategoryName: "Combustíveis", CategoryDescription: "Preços, tributação, composição e qualidade de combustíveis.",
			Title: "Etanol ou gasolina em Cuiabá", Excerpt: "Quando o etanol compensa.",
			BodyText: "Introdução.\n\nA regra dos 70%\n\n- Até 0,70: etanol.", PublishedAt: published, UpdatedAt: published.Add(time.Hour),
			Region: &forumdomain.Region{UF: "MT", MunicipalityIBGE: &cuiaba}, FuelProduct: &gasoline, StationID: &station,
			SourceName: "Equipe Abastevo",
		},
		{
			ID: "00000000-0000-4000-8000-0000000000f2", Category: forumdomain.CategoryAbastevoData,
			CategoryName: "Dados Abastevo", Title: "Contato com CPF", BodyText: "Fale com 123.456.789-09.",
			PublishedAt: published, UpdatedAt: published, SourceName: "Equipe Abastevo",
		},
		{
			ID: "00000000-0000-4000-8000-0000000000f3", Category: forumdomain.CategoryElectric,
			CategoryName: "Eletromobilidade", Title: "Recarga em rodovias", BodyText: "Texto sem região.",
			PublishedAt: published.Add(24 * time.Hour), UpdatedAt: published.Add(24 * time.Hour), SourceName: "Equipe Abastevo",
		},
	}}
}

func buildWithForum(t *testing.T, output string, forum ForumSource) Manifest {
	t.Helper()
	stations, prices := fixtureSources()
	builder := Builder{Stations: stations, Prices: prices, Forum: forum, PageSize: 2,
		Now: func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }}
	manifest, err := builder.Build(context.Background(), Options{
		FromSurvey: "2026-10-01", ToSurvey: "2026-10-31", OutputDir: output,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return manifest
}

func TestBuildWritesTheEditorialForumSection(t *testing.T) {
	output := filepath.Join(t.TempDir(), "snapshot")
	manifest := buildWithForum(t, output, forumFixture())
	if manifest.Counts.ForumPosts != 2 || manifest.Sources.Forum == nil ||
		*manifest.Sources.Forum != (ForumPin{Kind: ForumKind, Posts: 2, ExcludedPrivacy: 1}) {
		t.Fatalf("forum counts/pin = %+v / %+v", manifest.Counts, manifest.Sources.Forum)
	}
	if len(manifest.Files) != 4 || manifest.Files[3].Name != ForumFile || manifest.Files[3].Rows != 2 {
		t.Fatalf("files = %+v", manifest.Files)
	}
	raw, err := os.ReadFile(filepath.Join(output, ForumFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"123.456.789-09", "Contato com CPF", "author", "account", "alias"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("forum payload carries %q", forbidden)
		}
	}
	snapshot, err := ReadSnapshot(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ForumPosts) != 2 {
		t.Fatalf("forum posts = %+v", snapshot.ForumPosts)
	}
	want := ForumPost{
		PostID: "00000000-0000-4000-8000-0000000000f1", Category: "FUELS", CategoryName: "Combustíveis",
		CategoryDescription: "Preços, tributação, composição e qualidade de combustíveis.",
		Title:               "Etanol ou gasolina em Cuiabá", Excerpt: "Quando o etanol compensa.",
		Body:        "Introdução.\n\nA regra dos 70%\n\n- Até 0,70: etanol.",
		PublishedAt: "2026-11-09T12:00:00Z", UpdatedAt: "2026-11-09T13:00:00Z",
		RegionUF: "MT", RegionMunicipality: "5103403", FuelProduct: "GASOLINE_REGULAR",
		StationID: "00000000-0000-4000-8000-000000000001", SourceName: "Equipe Abastevo",
	}
	if snapshot.ForumPosts[0] != want {
		t.Errorf("forum row = %+v\nwant %+v", snapshot.ForumPosts[0], want)
	}
	if plain := snapshot.ForumPosts[1]; plain.RegionUF != "" || plain.RegionMunicipality != "" || plain.FuelProduct != "" || plain.StationID != "" {
		t.Errorf("plain row = %+v", plain)
	}
}

func TestForumChangesTheExportContentHash(t *testing.T) {
	parent := t.TempDir()
	withoutForum := buildFixture(t, filepath.Join(parent, "none"), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	if len(withoutForum.Files) != 3 || withoutForum.Sources.Forum != nil || withoutForum.Counts.ForumPosts != 0 {
		t.Fatalf("a build without a forum source changed shape: %+v", withoutForum)
	}
	base := buildWithForum(t, filepath.Join(parent, "base"), forumFixture())
	same := buildWithForum(t, filepath.Join(parent, "same"), forumFixture())
	if base.ContentSHA256 != same.ContentSHA256 {
		t.Fatal("identical forum input changed the content hash")
	}
	if base.ContentSHA256 == withoutForum.ContentSHA256 {
		t.Error("forum section did not change the content hash")
	}
	edited := forumFixture()
	edited.rows[0].Title = "Etanol ou gasolina em Cuiabá (atualizado)"
	hidden := forumFixture()
	hidden.rows = append(hidden.rows[:2:2], hidden.rows[3:]...)
	empty := &fakeForum{}
	for name, forum := range map[string]*fakeForum{"edit": edited, "hide": hidden, "empty": empty} {
		changed := buildWithForum(t, filepath.Join(parent, name), forum)
		if changed.ContentSHA256 == base.ContentSHA256 {
			t.Errorf("%s did not change the content hash", name)
		}
	}
	emptyManifest := buildWithForum(t, filepath.Join(parent, "empty-again"), empty)
	if emptyManifest.Sources.Forum == nil || emptyManifest.Sources.Forum.Posts != 0 || emptyManifest.ContentSHA256 == withoutForum.ContentSHA256 {
		t.Errorf("an empty forum section is not distinguishable from no forum: %+v", emptyManifest.Sources.Forum)
	}
}

func TestForumBudgetCursorAndInvalidRowsFailClosed(t *testing.T) {
	parent := t.TempDir()
	build := func(name string, forum ForumSource, max int) error {
		stations, prices := fixtureSources()
		builder := Builder{Stations: stations, Prices: prices, Forum: forum, PageSize: 2, MaxForumPosts: max}
		_, err := builder.Build(context.Background(), Options{
			FromSurvey: "2026-10-01", ToSurvey: "2026-10-31", OutputDir: filepath.Join(parent, name),
		})
		return err
	}
	if err := build("budget", forumFixture(), 1); err == nil || !strings.Contains(err.Error(), "forum budget") {
		t.Errorf("budget = %v", err)
	}
	stuck := forumFixture()
	stuck.stuck = true
	if err := build("stuck", stuck, 0); err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Errorf("stuck cursor = %v", err)
	}
	for name, mutate := range map[string]func(*forumapp.EditorialExportPost){
		"bad id":        func(p *forumapp.EditorialExportPost) { p.ID = "../x" },
		"blank title":   func(p *forumapp.EditorialExportPost) { p.Title = "  " },
		"no category":   func(p *forumapp.EditorialExportPost) { p.Category = "" },
		"no clock":      func(p *forumapp.EditorialExportPost) { p.PublishedAt = time.Time{} },
		"bad uf":        func(p *forumapp.EditorialExportPost) { p.Region = &forumdomain.Region{UF: "mt"} },
		"bad station":   func(p *forumapp.EditorialExportPost) { bad := "x"; p.StationID = &bad },
		"unsorted rows": nil,
	} {
		forum := forumFixture()
		if mutate == nil {
			forum.rows[0], forum.rows[2] = forum.rows[2], forum.rows[0]
		} else {
			mutate(&forum.rows[0])
		}
		if err := build(strings.ReplaceAll(name, " ", "-"), forum, 0); err == nil {
			t.Errorf("%s accepted", name)
		}
		if _, err := os.Stat(filepath.Join(parent, strings.ReplaceAll(name, " ", "-"))); !os.IsNotExist(err) {
			t.Errorf("%s left an output directory", name)
		}
	}
}

func TestReadSnapshotWithoutForumSection(t *testing.T) {
	output := filepath.Join(t.TempDir(), "snapshot")
	buildFixture(t, output, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	snapshot, err := ReadSnapshot(output)
	if err != nil || snapshot.ForumPosts != nil {
		t.Fatalf("snapshot without forum = %+v, %v", snapshot.ForumPosts, err)
	}
}
