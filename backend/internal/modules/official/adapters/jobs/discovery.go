package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// File name variants observed on the ANP page. Weeks run Sunday to Saturday.
// The current shape uses underscores inside each date and a hyphen between
// them; older weeks use ISO dates joined by an underscore. Both are admitted;
// the verified survey window comes from the filename, never from a label.
const (
	detailPattern  = "revendas_lpc_%s_%s.xlsx"
	summaryPattern = "resumo_semanal_lpc_%s_%s.xlsx"
	// The scheduled daily run keeps the historical two-week window. A
	// bounded correction sweep can request up to five weeks in its job
	// payload; older corrections still need a controlled backfill.
	defaultRecentWeeks = 2
	maxRecentWeeks     = 5
)

// Discovery probes bounded recent survey weeks and enqueues one import job per
// changed file content. Content hashing is intentional: the ANP source is not
// assumed to expose a reliable validator, so a replaced file is detected even
// when its URL or ETag did not change. Missing files (weeks not yet published)
// skip quietly; any other probe failure fails the job for bounded retry.
type Discovery struct {
	Fetch       *source.Fetcher
	ListingBase string
	Enqueue     func(ctx context.Context, payload ImportPayload, dedupe string) error
	Now         func() time.Time
}

// ImportPayload is the versioned import job envelope.
type ImportPayload struct {
	Version     int    `json:"version"`
	SourceURL   string `json:"source_url"`
	ETag        string `json:"etag"`
	SurveyStart string `json:"survey_start"`
	SurveyEnd   string `json:"survey_end"`
}

func (Discovery) Kind() string { return "anp-discovery" }
func (Discovery) Version() int { return 1 }

func (d Discovery) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// WeekStart returns the Sunday opening the week containing t.
func WeekStart(t time.Time) time.Time {
	y, m, day := t.Date()
	midnight := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	return midnight.AddDate(0, 0, -int(midnight.Weekday()))
}

func weekStrings(t time.Time) (start, end string) {
	s := WeekStart(t)
	return s.Format("2006-01-02"), s.AddDate(0, 0, 6).Format("2006-01-02")
}

// weekFileNames returns the admitted filename variants for one week, current
// shape first. A caller tries them in order and stops at the first existing
// resource, so a transition week never produces two imports.
func weekFileNames(pattern, start, end string) []string {
	currentPattern := strings.Replace(pattern, "%s_%s", "%s-%s", 1)
	current := fmt.Sprintf(currentPattern,
		replaceDateHyphens(start, "_"), replaceDateHyphens(end, "_"))
	legacy := fmt.Sprintf(pattern, start, end)
	if current == legacy {
		return []string{current}
	}
	return []string{current, legacy}
}

func replaceDateHyphens(date, replacement string) string {
	if len(date) != len("2006-01-02") {
		return date
	}
	return date[0:4] + replacement + date[5:7] + replacement + date[8:10]
}

type discoveryPayload struct {
	Weeks int `json:"weeks"`
}

// Handle probes recent survey weeks for both file kinds. The scheduled run
// covers the current and previous week; an operator-triggered correction sweep
// may request up to five weeks in the job payload. A 404 means the week (or
// that filename variant) is not published and skips quietly; any other probe
// failure fails the job for retry. The content hash is the dedupe identity, so
// unchanged bytes converge and replaced bytes create one new job.
func (d Discovery) Handle(ctx context.Context, job jobs.Job) error {
	if d.Fetch == nil {
		return fmt.Errorf("discovery: no fetcher configured")
	}
	if d.Enqueue == nil {
		return fmt.Errorf("discovery: no enqueue configured")
	}
	weeks := defaultRecentWeeks
	if len(job.Payload) > 0 {
		var payload discoveryPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("discovery: bad payload: %w", err)
		}
		if payload.Weeks > 0 {
			weeks = payload.Weeks
			if weeks > maxRecentWeeks {
				weeks = maxRecentWeeks
			}
		}
	}
	now := d.now()
	for week := 0; week < weeks; week++ {
		ref := now.AddDate(0, 0, -7*week)
		start, end := weekStrings(ref)
		year := start[:4]
		for _, pattern := range []string{detailPattern, summaryPattern} {
			if err := d.probeWeekFile(ctx, pattern, year, start, end); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d Discovery) probeWeekFile(ctx context.Context, pattern, year, start, end string) error {
	for _, name := range weekFileNames(pattern, start, end) {
		rawURL := d.ListingBase + year + "/" + name
		changed, _, _, sha, err := d.Fetch.ProbeContent(ctx, rawURL, "")
		if err != nil {
			if errors.Is(err, source.ErrNotFound) {
				continue
			}
			return err
		}
		if !changed || sha == "" {
			continue
		}
		// The import fetches the bytes again and BeginRun treats an identical
		// checksum as a no-op. Leaving ETag empty avoids a conditional 304
		// hiding content that changed after this probe.
		return d.Enqueue(ctx, ImportPayload{
			Version: 1, SourceURL: rawURL,
			SurveyStart: start, SurveyEnd: end,
		}, "anp-import:"+rawURL+":"+sha)
	}
	return nil
}
