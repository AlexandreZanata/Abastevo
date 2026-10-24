package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const asaasUserAgent = "Abastevo/1.0"

type AsaasProvider struct {
	BaseURL     string
	APIKey      string
	Environment domain.Environment
	HTTPClient  *http.Client
	Now         func() time.Time
}

func NewAsaasProvider(apiKey string, environment domain.Environment, httpClient *http.Client) *AsaasProvider {
	baseURL := "https://api-sandbox.asaas.com"
	if environment == domain.EnvironmentProduction {
		baseURL = "https://api.asaas.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &AsaasProvider{
		BaseURL: baseURL, APIKey: apiKey, Environment: environment,
		HTTPClient: httpClient, Now: time.Now,
	}
}

func (p *AsaasProvider) EnsureCustomer(ctx context.Context, request app.PixCustomerRequest) (app.Customer, error) {
	customers, complete, err := p.findCustomers(ctx, request.AccountID)
	if err != nil {
		return app.Customer{}, err
	}
	if len(customers) > 1 {
		return app.Customer{}, domain.ErrConflict
	}
	if len(customers) == 1 {
		customer := customers[0]
		if customer.ID == "" || customer.ExternalReference != request.AccountID {
			return app.Customer{}, domain.ErrBinding
		}
		return app.Customer{ID: customer.ID}, nil
	}
	if !complete {
		return app.Customer{}, app.ErrAmbiguous
	}
	body := map[string]any{
		"name": request.Name, "cpfCnpj": request.TaxID,
		"externalReference": request.AccountID, "notificationDisabled": true,
	}
	if request.Email != "" {
		body["email"] = request.Email
	}
	var created asaasCustomer
	if err := p.do(ctx, http.MethodPost, "/v3/customers", body, &created); err != nil {
		return app.Customer{}, err
	}
	if created.ID == "" || created.ExternalReference != request.AccountID {
		return app.Customer{}, domain.ErrBinding
	}
	return app.Customer{ID: created.ID}, nil
}

func (p *AsaasProvider) CreatePayment(ctx context.Context, request app.PixPaymentRequest) (app.PixPayment, error) {
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return app.PixPayment{}, domain.ErrUnavailable
	}
	body := map[string]any{
		"customer": request.CustomerID, "billingType": "PIX",
		"value":             json.Number(request.Order.Amount.String()),
		"dueDate":           request.DueDate.In(location).Format("2006-01-02"),
		"externalReference": request.Order.ID, "description": "Abastevo Premium",
	}
	var payment asaasPayment
	if err := p.do(ctx, http.MethodPost, "/v3/payments", body, &payment); err != nil {
		return app.PixPayment{}, err
	}
	return mapAsaasPayment(payment, location)
}

func (p *AsaasProvider) RetrievePayment(ctx context.Context, paymentID string) (app.PixPayment, error) {
	if !validAsaasID(paymentID) {
		return app.PixPayment{}, domain.ErrInvalid
	}
	var payment asaasPayment
	if err := p.do(ctx, http.MethodGet, "/v3/payments/"+url.PathEscape(paymentID), nil, &payment); err != nil {
		return app.PixPayment{}, err
	}
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return app.PixPayment{}, domain.ErrUnavailable
	}
	return mapAsaasPayment(payment, location)
}

func (p *AsaasProvider) FindPayment(ctx context.Context, externalReference string) (app.PixPayment, bool, bool, error) {
	if externalReference == "" || len(externalReference) > 128 {
		return app.PixPayment{}, false, false, domain.ErrInvalid
	}
	var list asaasPaymentList
	query := url.Values{"externalReference": {externalReference}, "limit": {"100"}}
	if err := p.do(ctx, http.MethodGet, "/v3/payments?"+query.Encode(), nil, &list); err != nil {
		return app.PixPayment{}, false, false, err
	}
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return app.PixPayment{}, false, false, domain.ErrUnavailable
	}
	var matches []asaasPayment
	for _, payment := range list.Data {
		if payment.ExternalReference == externalReference {
			matches = append(matches, payment)
		}
	}
	if len(matches) > 1 {
		return app.PixPayment{}, false, false, domain.ErrConflict
	}
	if len(matches) == 1 {
		result, err := mapAsaasPayment(matches[0], location)
		if err != nil {
			return app.PixPayment{}, false, false, err
		}
		return result, true, true, nil
	}
	complete := !list.HasMore && list.TotalCount <= len(list.Data)
	return app.PixPayment{}, false, complete, nil
}

func (p *AsaasProvider) GetQR(ctx context.Context, paymentID string) (app.PixQR, error) {
	if !validAsaasID(paymentID) {
		return app.PixQR{}, domain.ErrInvalid
	}
	var response asaasQR
	if err := p.do(ctx, http.MethodGet, "/v3/payments/"+url.PathEscape(paymentID)+"/pixQrCode", nil, &response); err != nil {
		return app.PixQR{}, err
	}
	if response.Payload == "" || len(response.Payload) > 4096 ||
		response.EncodedImage == "" || len(response.EncodedImage) > 64*1024 {
		return app.PixQR{}, domain.ErrBinding
	}
	result := app.PixQR{Payload: response.Payload, Image: response.EncodedImage}
	if response.ExpirationDate != "" {
		location, err := time.LoadLocation(domain.BillingTimezone)
		if err != nil {
			return app.PixQR{}, domain.ErrUnavailable
		}
		for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
			if parsed, err := time.ParseInLocation(layout, response.ExpirationDate, location); err == nil {
				result.ExpiresAt = parsed.UTC()
				break
			}
		}
	}
	return result, nil
}

func (p *AsaasProvider) findCustomers(ctx context.Context, externalReference string) ([]asaasCustomer, bool, error) {
	var list asaasCustomerList
	query := url.Values{"externalReference": {externalReference}, "limit": {"100"}}
	if err := p.do(ctx, http.MethodGet, "/v3/customers?"+query.Encode(), nil, &list); err != nil {
		return nil, false, err
	}
	var matches []asaasCustomer
	for _, customer := range list.Data {
		if customer.ExternalReference == externalReference {
			matches = append(matches, customer)
		}
	}
	complete := !list.HasMore && list.TotalCount <= len(list.Data)
	return matches, complete, nil
}

func (p *AsaasProvider) do(ctx context.Context, method, path string, body any, out any) error {
	if p == nil || p.HTTPClient == nil || p.APIKey == "" || p.BaseURL == "" {
		return domain.ErrUnavailable
	}
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return domain.ErrInvalid
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.BaseURL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return domain.ErrInvalid
	}
	request.Header.Set("access_token", p.APIKey)
	request.Header.Set("User-Agent", asaasUserAgent)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := p.HTTPClient.Do(request)
	if err != nil {
		return app.ErrAmbiguous
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return asaasStatusError(response.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := decoder.Decode(out); err != nil {
		return app.ErrAmbiguous
	}
	return nil
}

func asaasStatusError(status int) error {
	switch {
	case status == http.StatusBadRequest, status == http.StatusUnprocessableEntity:
		return domain.ErrInvalid
	case status == http.StatusNotFound:
		return domain.ErrNotFound
	case status == http.StatusConflict:
		return domain.ErrConflict
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return domain.ErrUnavailable
	case status == http.StatusTooManyRequests, status >= 500:
		return app.ErrAmbiguous
	default:
		return domain.ErrUnavailable
	}
}

func mapAsaasPayment(payment asaasPayment, location *time.Location) (app.PixPayment, error) {
	if payment.ID == "" || payment.Customer == "" || payment.ExternalReference == "" ||
		payment.Status == "" || payment.Value.String() == "" {
		return app.PixPayment{}, domain.ErrBinding
	}
	amount, err := parseBRLMinor(payment.Value)
	if err != nil {
		return app.PixPayment{}, domain.ErrBinding
	}
	result := app.PixPayment{
		ID: payment.ID, CustomerID: payment.Customer, ExternalReference: payment.ExternalReference,
		Amount: domain.BRL(amount), Status: payment.Status, ProviderStatus: payment.Status,
		Class: asaasPixClass(payment.Status),
	}
	if payment.DueDate != "" {
		if due, err := time.ParseInLocation("2006-01-02", payment.DueDate, location); err == nil {
			result.DueDate = due.UTC()
		}
	}
	return result, nil
}

func asaasPixClass(status string) string {
	switch status {
	case "RECEIVED":
		return app.PixSettled
	case "PENDING", "AWAITING_RISK_ANALYSIS", "CONFIRMED":
		return app.PixPending
	case "OVERDUE":
		return app.PixExpired
	case "REFUND_REQUESTED", "REFUND_IN_PROGRESS", "REFUNDED":
		return app.PixRefunded
	case "CHARGEBACK_REQUESTED", "CHARGEBACK_DISPUTE", "AWAITING_CHARGEBACK_REVERSAL":
		return app.PixDisputed
	default:
		return app.PixUnknown
	}
}

func parseBRLMinor(value json.Number) (int64, error) {
	raw := value.String()
	if raw == "" || strings.HasPrefix(raw, "-") || strings.ContainsAny(raw, "eE+") {
		return 0, errors.New("invalid BRL value")
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("invalid BRL value")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 || whole > 1_000_000_000 {
		return 0, errors.New("invalid BRL value")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) > 2 {
			return 0, errors.New("invalid BRL value")
		}
		for len(fraction) < 2 {
			fraction += "0"
		}
	}
	cents := int64(0)
	if fraction != "" {
		cents, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil || cents < 0 || cents > 99 {
			return 0, errors.New("invalid BRL value")
		}
	}
	return whole*100 + cents, nil
}

func validAsaasID(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for _, r := range value {
		if r <= 32 || r > 126 || r == '/' {
			return false
		}
	}
	return true
}

type asaasCustomer struct {
	ID                string `json:"id"`
	ExternalReference string `json:"externalReference"`
}

type asaasCustomerList struct {
	Data       []asaasCustomer `json:"data"`
	TotalCount int             `json:"totalCount"`
	HasMore    bool            `json:"hasMore"`
}

type asaasPayment struct {
	ID                string      `json:"id"`
	Customer          string      `json:"customer"`
	Status            string      `json:"status"`
	ExternalReference string      `json:"externalReference"`
	Value             json.Number `json:"value"`
	DueDate           string      `json:"dueDate"`
}

type asaasPaymentList struct {
	Data       []asaasPayment `json:"data"`
	TotalCount int            `json:"totalCount"`
	HasMore    bool           `json:"hasMore"`
}

type asaasQR struct {
	EncodedImage   string `json:"encodedImage"`
	Payload        string `json:"payload"`
	ExpirationDate string `json:"expirationDate"`
}

var _ app.PixProvider = (*AsaasProvider)(nil)
