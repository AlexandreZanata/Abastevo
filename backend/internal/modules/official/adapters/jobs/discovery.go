package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// File names from docs/data-sources.md. Weeks run Sunday to Saturday.
const (
	detailPattern  = "revendas_lpc_%s_%s.xlsx"
	summaryPattern = "resumo_semanal_lpc_%s_%s.xlsx"
)

// Discovery probes the documented weekly file URLs for recent survey weeks
// and enqueues one import job per changed etag. Unchanged etags converge on
// the existing job row instead of duplicating work; missing files (weeks
// not yet published) skip quietly.
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

// Handle probes the current and previous survey weeks for both file kinds.
// A 404 means the week is not published yet and skips quietly; any other
// probe failure fails the job for retry. Sources without an ETag converge
// on one import job per URL and re-import only through manual enqueue.
func (d Discovery) Handle(ctx context.Context, job jobs.Job) error {
	if d.Fetch == nil {
		return fmt.Errorf("discovery: no fetcher configured")
	}
	if d.Enqueue == nil {
		return fmt.Errorf("discovery: no enqueue configured")
	}
	now := d.now()
	for _, ref := range []time.Time{now, now.AddDate(0, 0, -7)} {
		start, end := weekStrings(ref)
		year := start[:4]
		for _, name := range []string{
			fmt.Sprintf(detailPattern, start, end),
			fmt.Sprintf(summaryPattern, start, end),
		} {
			rawURL := d.ListingBase + year + "/" + name
			changed, etag, _, err := d.Fetch.Probe(ctx, rawURL, "")
			if err != nil {
				if errors.Is(err, source.ErrBadStatus) && strings.Contains(err.Error(), "404") {
					continue
				}
				return err
			}
			if !changed {
				continue
			}
			if err := d.Enqueue(ctx, ImportPayload{
				Version: 1, SourceURL: rawURL, ETag: etag,
				SurveyStart: start, SurveyEnd: end,
			}, "anp-import:"+rawURL+":"+etag); err != nil {
				return err
			}
		}
	}
	return nil
}
