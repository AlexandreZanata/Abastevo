package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/anp"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	odomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// ParserVersion versions the parse-normalize pipeline: same bytes under a
// new version reprocess traceably instead of no-op.
const ParserVersion = "anp-v1"

// Import fetches one discovered file and publishes it through the existing
// pipeline: fetch, parse, kernel normalization, station resolution, staged
// batching and atomic publication. Published and needs-review verdicts both
// complete the job (review state is data, not failure); empty or broken
// sources fail toward DEAD with cause.
type Import struct {
	Fetch   *source.Fetcher
	Parser  anp.Parser
	Import  *adapters.Importer
	Resolve func(ctx context.Context, cnpjNormalized, displayName string, address map[string]string) (string, error)
}

// Kind implements jobs.Handler.
func (Import) Kind() string { return "anp-import" }

// Version implements jobs.Handler.
func (Import) Version() int { return 1 }

func (im Import) Handle(ctx context.Context, job jobs.Job) error {
	var payload ImportPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("import: bad payload: %w", err)
	}
	if payload.Version != 1 || payload.SourceURL == "" {
		return fmt.Errorf("import: bad payload")
	}
	surveyStart, err := time.Parse("2006-01-02", payload.SurveyStart)
	if err != nil {
		return fmt.Errorf("import: bad survey window: %w", err)
	}
	surveyEnd, err := time.Parse("2006-01-02", payload.SurveyEnd)
	if err != nil {
		return fmt.Errorf("import: bad survey window: %w", err)
	}
	res, err := im.Fetch.Fetch(ctx, payload.SourceURL, payload.ETag, "")
	if err != nil {
		return err
	}
	defer func() { _ = res.Close() }()
	if res.NotModified {
		return nil
	}
	raw, err := os.ReadFile(res.Path)
	if err != nil {
		return err
	}
	rows, err := parseRows(im.Parser, raw)
	if err != nil {
		return err
	}
	begun, err := im.Import.BeginRun(ctx, odomain.ImportKey{
		SourceURL: payload.SourceURL, SourceChecksum: res.SHA256, ParserVersion: ParserVersion,
	}, surveyStart, surveyEnd)
	if err != nil {
		return err
	}
	if begun.NoOp {
		return nil
	}
	var tally odomain.QuarantineTally
	var staged int64
	batch := make([]odomain.PriceRow, 0, 500)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := im.Import.StageBatch(ctx, begun.RevisionID, batch, &tally)
		staged += n
		batch = batch[:0]
		return err
	}
	for _, row := range rows {
		prow, ok := normalizeRow(row, &tally)
		if !ok {
			continue
		}
		stationID, err := im.Resolve(ctx, prow.CNPJ, prow.Display, prow.Address)
		if err != nil {
			tally.Add("unresolved-station", prow.CNPJ)
			continue
		}
		prow.Row.StationID = stationID
		batch = append(batch, prow.Row)
		if len(batch) >= 500 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	_ = staged
	fin, err := im.Import.FinishRun(ctx, begun.RunID, begun.RevisionID, &tally)
	if err != nil {
		return err
	}
	if !fin.Published && fin.Verdict.Reason == "empty" {
		return fmt.Errorf("import: empty source")
	}
	return nil
}

type normalizedRow struct {
	Row     odomain.PriceRow
	CNPJ    string
	Display string
	Address map[string]string
}

// normalizeRow maps one parsed row through kernel values. Anything unproven
// tallies with the fixture reason codes; only exact values stage.
func normalizeRow(row anp.Row, tally *odomain.QuarantineTally) (normalizedRow, bool) {
	var out normalizedRow
	label := row.Cells["PRODUTO"]
	product, err := kernel.ParseProduct(label)
	if err != nil {
		tally.Add(kernel.QuarantineCode(err), row.Cells["CNPJ"])
		return out, false
	}
	unit, err := product.Unit()
	if err != nil {
		tally.Add(kernel.QuarantineCode(err), row.Cells["CNPJ"])
		return out, false
	}
	price, err := kernel.ParsePrice(product, unit, row.Cells["PRECO"])
	if err != nil {
		tally.Add(kernel.QuarantineCode(err), row.Cells["CNPJ"])
		return out, false
	}
	cnpj, err := kernel.ParseCNPJ(row.Cells["CNPJ"])
	if err != nil {
		tally.Add("cnpj-checksum-invalid", row.Cells["CNPJ"])
		return out, false
	}
	collected, ok := parseCollected(row.Cells["DATA"], tally)
	if !ok {
		return out, false
	}
	display := firstPresent(row.Cells, "NOME FANTASIA", "RAZAO SOCIAL", "POSTO", "REVENDEDOR")
	if display == "" {
		display = "Posto " + cnpj.Normalized()
	}
	out = normalizedRow{
		Row: odomain.PriceRow{
			Product: string(product), Unit: string(unit),
			AmountMilli: price.Milli, RawText: price.Raw,
			CollectedOn: collected, SourceRow: row.Number,
		},
		CNPJ:    cnpj.Normalized(),
		Display: display,
		Address: map[string]string{},
	}
	return out, true
}

func firstPresent(cells map[string]string, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(cells[n]); v != "" {
			return v
		}
	}
	return ""
}

// parseCollected accepts Excel serials and ISO dates; anything else
// quarantines with cause instead of guessing.
func parseCollected(text string, tally *odomain.QuarantineTally) (t time.Time, ok bool) {
	if text = strings.TrimSpace(text); text == "" {
		tally.Add("missing-date", "")
		return time.Time{}, false
	} else if serial, err := strconv.Atoi(text); err == nil {
		iso, err := anp.ExcelSerialToDate(serial)
		if err != nil {
			tally.Add("invalid-date", text)
			return time.Time{}, false
		}
		day, _ := time.Parse("2006-01-02", iso)
		return day, true
	} else if day, err := time.Parse("2006-01-02", text); err == nil {
		return day, true
	} else {
		tally.Add("invalid-date", text)
		return time.Time{}, false
	}
}

func parseRows(p anp.Parser, raw []byte) ([]anp.Row, error) {
	var rows []anp.Row
	_, _, err := p.Parse(raw, "", func(r anp.Row) error {
		rows = append(rows, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}
