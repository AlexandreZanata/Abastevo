package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// decodeAPIPage recognizes the official envelope, never an arbitrary object.
// Actual ANP pagination has bounded numbered pages and explicit row counts.
func decodeAPIPage(body []byte, pageNumber int) (apiPage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return apiPage{}, err
	}
	if _, ok := envelope["data"]; !ok {
		return apiPage{}, errors.New("registry: missing official data")
	}
	var wire struct {
		Title     string            `json:"title"`
		Status    int               `json:"status"`
		Succeeded bool              `json:"succeeded"`
		Data      []json.RawMessage `json:"data"`
		Filter    *struct {
			Number int `json:"numeroPagina"`
			Size   int `json:"tamanhoPagina"`
			Total  int `json:"totalRegistro"`
			Pages  int `json:"totalPagina"`
		} `json:"searchPageFilter"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return apiPage{}, err
	}
	f := wire.Filter
	// ANP's observed no-results envelope omits pagination. Recognize its
	// explicit success/title only; a truncated or arbitrary object is not empty.
	if wire.Status == 200 && wire.Succeeded && wire.Data != nil && len(wire.Data) == 0 && f == nil && wire.Title == "Nenhum dado encontrado" {
		return apiPage{Items: wire.Data, Official: true, Total: 0, Pages: 0}, nil
	}
	if wire.Status != 200 || !wire.Succeeded || wire.Data == nil || f == nil || f.Number != pageNumber || f.Size <= 0 || f.Size > 5000 || f.Total < 0 || f.Pages < 0 || len(wire.Data) > f.Size {
		return apiPage{}, errors.New("registry: invalid official envelope")
	}
	expectedPages := (f.Total + f.Size - 1) / f.Size
	// Empty result envelopes may report zero or one page, never additional pages.
	if f.Total == 0 && f.Pages == 1 {
		expectedPages = 1
	}
	if f.Pages != expectedPages || (f.Total > 0 && pageNumber > f.Pages) {
		return apiPage{}, errors.New("registry: inconsistent pagination")
	}
	remaining := f.Total - (pageNumber-1)*f.Size
	expectedRows := min(f.Size, max(0, remaining))
	if len(wire.Data) != expectedRows {
		return apiPage{}, errors.New("registry: incomplete official page")
	}
	page := apiPage{Items: wire.Data, Official: true, Total: f.Total, Pages: f.Pages}
	if f.Number < f.Pages {
		next := strconv.Itoa(f.Number + 1)
		page.NextCursor = &next
	}
	return page, nil
}

// Public regulatory/geographic metadata only. Free-text inspection notes and
// people are deliberately absent. Metre accuracy is a source claim, not review.
type officialAPIRecord struct {
	CNPJ          string `json:"cnpj"`
	Business      string `json:"razaoSocial"`
	SIMP          string `json:"codigoSIMP"`
	Authorization string `json:"autorizacao"`
	Published     string `json:"dataPublicacao"`
	Address       string `json:"endereco"`
	Complement    string `json:"complemento"`
	District      string `json:"bairro"`
	PostalCode    string `json:"cep"`
	State         string `json:"uf"`
	Municipality  string `json:"municipio"`
	Latitude      string `json:"latitude"`
	Longitude     string `json:"longitude"`
	SRID          string `json:"srid"`
	CRS           string `json:"src"`
	Validation    string `json:"validacao"`
	Accuracy      string `json:"estimativaAcuracia"`
	Obtained      string `json:"dataObtencao"`
	Origin        string `json:"origemInformacao"`
}

func parseOfficialAPIRecord(raw json.RawMessage) (Assertion, bool) {
	var rec officialAPIRecord
	if json.Unmarshal(raw, &rec) != nil {
		return Assertion{}, false
	}
	cnpj, err := ValidCNPJText(rec.CNPJ)
	name, uf := strings.TrimSpace(rec.Business), strings.ToUpper(strings.TrimSpace(rec.State))
	if err != nil || name == "" || len(uf) != 2 || strings.TrimSpace(rec.Address) == "" || strings.TrimSpace(rec.Municipality) == "" {
		return Assertion{}, false
	}
	a := Assertion{Source: SourceAPI, SourceKey: cnpj, DisplayName: name, State: uf,
		AuthState: "unknown", Eligibility: "pending", LocationQuality: "unknown",
		Address: map[string]string{"raw": rec.Address, "complement": rec.Complement, "district": rec.District, "postal_code": rec.PostalCode, "municipio": rec.Municipality, "uf": uf},
	}
	lat, lon := strings.TrimSpace(rec.Latitude), strings.TrimSpace(rec.Longitude)
	if (lat == "") != (lon == "") {
		return Assertion{}, false
	}
	if lat != "" {
		latitude, e1 := strconv.ParseFloat(lat, 64)
		longitude, e2 := strconv.ParseFloat(lon, 64)
		if e1 != nil || e2 != nil || !(latitude >= -34 && latitude <= 6 && longitude >= -74 && longitude <= -28) {
			return Assertion{}, false
		}
		switch {
		case rec.SRID == "4674" && strings.EqualFold(rec.CRS, "SIRGAS2000"):
			a.CRS = "SIRGAS2000"
		case rec.SRID == "4326" && strings.EqualFold(rec.CRS, "WGS84"):
			a.CRS = "WGS84"
		default:
			return Assertion{}, false
		}
		a.Latitude, a.Longitude, a.HasCoords = latitude, longitude, true
	}
	if rec.Obtained != "" {
		date, err := time.Parse("02/01/2006", rec.Obtained)
		if err != nil {
			return Assertion{}, false
		}
		a.EffectiveDate = pgtype.Date{Time: date, Valid: true}
	}
	// Preserve validation/accuracy/date/origin with the exact source identity.
	// JSON is bounded by the fetch limit; no inspection free text is retained.
	provenance, _ := json.Marshal(struct {
		Parser        string `json:"parser"`
		SIMP          string `json:"simp"`
		Authorization string `json:"authorization"`
		Validation    string `json:"validation"`
		Accuracy      string `json:"accuracy_m_claim"`
		Obtained      string `json:"obtained_at"`
		Origin        string `json:"origin"`
		SRID          string `json:"srid"`
	}{APIParserVersion, rec.SIMP, rec.Authorization, rec.Validation, rec.Accuracy, rec.Obtained, rec.Origin, rec.SRID})
	a.SourceReference = fmt.Sprintf("https://revendedoresapi.anp.gov.br/v1/combustivel?cnpj=%s#%s", cnpj, provenance)
	canonical, _ := json.Marshal(rec)
	sum := sha256.Sum256(canonical)
	a.Checksum = hex.EncodeToString(sum[:])
	return a, true
}
