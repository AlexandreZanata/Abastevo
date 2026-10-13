package http

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
)

func errInvalidReport() error { return errors.New("ledger: invalid input") }

func csvEscape(field string) string {
	formula := strings.HasPrefix(field, "=") || strings.HasPrefix(field, "+") || strings.HasPrefix(field, "-") || strings.HasPrefix(field, "@")
	if !strings.ContainsAny(field, "\"\n\r") && !formula {
		return field
	}
	escaped := strings.ReplaceAll(field, "\"", "\"\"")
	if formula {
		escaped = "'" + escaped
	}
	return "\"" + escaped + "\""
}

func reportCSVBody(report application.Report) string {
	var b strings.Builder
	b.WriteString("year,month,timezone,revision,metric,value\n")
	fmt.Fprintf(&b, "%d,%d,%s,%d,%s,%d\n", report.Year, report.Month, csvEscape(report.Timezone), report.Revision, csvEscape("purchase_centavos"), report.Totals.PurchaseCentavos)
	fmt.Fprintf(&b, "%d,%d,%s,%d,%s,%d\n", report.Year, report.Month, csvEscape(report.Timezone), report.Revision, csvEscape("expense_centavos"), report.Totals.ExpenseCentavos)
	vehicles := make([]string, 0, len(report.Totals.ByVehicle))
	for vehicle := range report.Totals.ByVehicle {
		vehicles = append(vehicles, vehicle)
	}
	sort.Strings(vehicles)
	for _, vehicle := range vehicles {
		fmt.Fprintf(&b, "%d,%d,%s,%d,%s,%d\n", report.Year, report.Month, csvEscape(report.Timezone), report.Revision, csvEscape("vehicle:"+vehicle), report.Totals.ByVehicle[vehicle])
	}
	categories := make([]string, 0, len(report.Totals.ByCategory))
	for category := range report.Totals.ByCategory {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	for _, category := range categories {
		fmt.Fprintf(&b, "%d,%d,%s,%d,%s,%d\n", report.Year, report.Month, csvEscape(report.Timezone), report.Revision, csvEscape("category:"+category), report.Totals.ByCategory[category])
	}
	return b.String()
}

func validReportParams(year, month int, timezone string) bool {
	return year >= 2000 && year <= 2100 && month >= 1 && month <= 12 && len(timezone) >= 1 && len(timezone) <= 64
}

func (h Handler) reportBuild(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Year     int    `json:"year"`
		Month    int    `json:"month"`
		Timezone string `json:"timezone"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if !validReportParams(dto.Year, dto.Month, dto.Timezone) {
		fail(w, r, errInvalidReport())
		return
	}
	owner, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	got, err := h.Service.BuildReport(r.Context(), owner, dto.Year, dto.Month, dto.Timezone)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}

func (h Handler) reportGet(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Year     int    `json:"year"`
		Month    int    `json:"month"`
		Timezone string `json:"timezone"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if !validReportParams(dto.Year, dto.Month, dto.Timezone) {
		fail(w, r, errInvalidReport())
		return
	}
	owner, err := h.principal(r, dto.session, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	got, err := h.Service.GetReport(r.Context(), owner, dto.Year, dto.Month, dto.Timezone)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}

func (h Handler) reportCSV(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Year     int    `json:"year"`
		Month    int    `json:"month"`
		Timezone string `json:"timezone"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if !validReportParams(dto.Year, dto.Month, dto.Timezone) {
		fail(w, r, errInvalidReport())
		return
	}
	owner, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	got, err := h.Service.BuildReport(r.Context(), owner, dto.Year, dto.Month, dto.Timezone)
	if err != nil {
		fail(w, r, err)
		return
	}
	body := reportCSVBody(got)
	w.Header().Del("ETag")
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(body))
}
