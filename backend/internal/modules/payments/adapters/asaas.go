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

func (p *AsaasProvider) CreatePixAutoAuthorization(ctx context.Context, request app.PixAutoProviderRequest) (app.PixAutoAuthorization, error) {
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return app.PixAutoAuthorization{}, domain.ErrUnavailable
	}
	body := map[string]any{
		"frequency": "MONTHLY", "contractId": request.ContractID,
		"startDate":  request.StartDate.In(location).Format("2006-01-02"),
		"customerId": request.CustomerID, "value": json.Number(request.Value.String()),
		"paymentCreationMode": "SUBSCRIPTION", "retryPolicy": "NOT_ALLOWED",
		"description": "Abastevo Premium",
		"immediateQrCode": map[string]any{
			"expirationSeconds": int64(request.ImmediateExpiration / time.Second),
			"originalValue":     json.Number(request.Value.String()),
		},
	}
	if !request.FinishDate.IsZero() {
		body["finishDate"] = request.FinishDate.In(location).Format("2006-01-02")
	}
	var response asaasPixAuto
	if err := p.do(ctx, http.MethodPost, "/v3/pix/automatic/authorizations", body, &response); err != nil {
		return app.PixAutoAuthorization{}, err
	}
	return mapPixAuto(response, location)
}

func (p *AsaasProvider) RetrievePixAutoInstruction(ctx context.Context, id string) (app.PixAutoInstruction, error) {
	if !validAsaasID(id) {
		return app.PixAutoInstruction{}, domain.ErrInvalid
	}
	var response asaasPixAutoInstruction
	if err := p.do(ctx, http.MethodGet, "/v3/pix/automatic/paymentInstructions/"+url.PathEscape(id), nil, &response); err != nil {
		return app.PixAutoInstruction{}, err
	}
	return mapPixAutoInstruction(response)
}

func (p *AsaasProvider) ListPixAutoInstructions(ctx context.Context, authorizationID string) ([]app.PixAutoInstruction, bool, error) {
	if !validAsaasID(authorizationID) {
		return nil, false, domain.ErrInvalid
	}
	var list asaasPixAutoInstructionList
	query := url.Values{"authorizationId": {authorizationID}, "limit": {"100"}}
	if err := p.do(ctx, http.MethodGet, "/v3/pix/automatic/paymentInstructions?"+query.Encode(), nil, &list); err != nil {
		return nil, false, err
	}
	instructions := make([]app.PixAutoInstruction, 0, len(list.Data))
	for _, item := range list.Data {
		mapped, err := mapPixAutoInstruction(item)
		if err != nil {
			return nil, false, err
		}
		instructions = append(instructions, mapped)
	}
	complete := !list.HasMore && list.TotalCount <= len(list.Data)
	return instructions, complete, nil
}

func (p *AsaasProvider) ListSubscriptionPayments(ctx context.Context, subscriptionID string) ([]app.PixPayment, bool, error) {
	if !validAsaasID(subscriptionID) {
		return nil, false, domain.ErrInvalid
	}
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return nil, false, domain.ErrUnavailable
	}
	var list asaasPaymentList
	query := url.Values{"subscription": {subscriptionID}, "status": {"RECEIVED"}, "limit": {"100"}}
	if err := p.do(ctx, http.MethodGet, "/v3/payments?"+query.Encode(), nil, &list); err != nil {
		return nil, false, err
	}
	payments := make([]app.PixPayment, 0, len(list.Data))
	for _, payment := range list.Data {
		mapped, err := mapAsaasPayment(payment, location)
		if err != nil {
			return nil, false, err
		}
		payments = append(payments, mapped)
	}
	complete := !list.HasMore && list.TotalCount <= len(list.Data)
	return payments, complete, nil
}

func (p *AsaasProvider) RetrievePixAutoAuthorization(ctx context.Context, id string) (app.PixAutoAuthorization, error) {
	if !validAsaasID(id) {
		return app.PixAutoAuthorization{}, domain.ErrInvalid
	}
	var response asaasPixAuto
	if err := p.do(ctx, http.MethodGet, "/v3/pix/automatic/authorizations/"+url.PathEscape(id), nil, &response); err != nil {
		return app.PixAutoAuthorization{}, err
	}
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return app.PixAutoAuthorization{}, domain.ErrUnavailable
	}
	return mapPixAuto(response, location)
}

func (p *AsaasProvider) FindPixAutoAuthorization(ctx context.Context, customerID, contractID string) (app.PixAutoAuthorization, bool, bool, error) {
	if !validAsaasID(customerID) || contractID == "" {
		return app.PixAutoAuthorization{}, false, false, domain.ErrInvalid
	}
	var list asaasPixAutoList
	query := url.Values{"customerId": {customerID}, "limit": {"100"}}
	if err := p.do(ctx, http.MethodGet, "/v3/pix/automatic/authorizations?"+query.Encode(), nil, &list); err != nil {
		return app.PixAutoAuthorization{}, false, false, err
	}
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return app.PixAutoAuthorization{}, false, false, domain.ErrUnavailable
	}
	var matches []asaasPixAuto
	for _, item := range list.Data {
		if item.ContractID == contractID {
			matches = append(matches, item)
		}
	}
	if len(matches) > 1 {
		return app.PixAutoAuthorization{}, false, false, domain.ErrConflict
	}
	if len(matches) == 1 {
		result, err := mapPixAuto(matches[0], location)
		if err != nil {
			return app.PixAutoAuthorization{}, false, false, err
		}
		return result, true, true, nil
	}
	complete := !list.HasMore && list.TotalCount <= len(list.Data)
	return app.PixAutoAuthorization{}, false, complete, nil
}

func (p *AsaasProvider) CancelPixAutoAuthorization(ctx context.Context, id string) error {
	if !validAsaasID(id) {
		return domain.ErrInvalid
	}
	return p.do(ctx, http.MethodDelete, "/v3/pix/automatic/authorizations/"+url.PathEscape(id), nil, nil)
}

func (p *AsaasProvider) VerifySettlement(ctx context.Context, event app.InboxRecord) (app.SettlementCandidate, error) {
	if event.EventType != "PAYMENT_RECEIVED" || event.ResourceType != "payment" {
		return app.SettlementCandidate{}, app.ErrIgnored
	}
	payment, err := p.RetrievePayment(ctx, event.ResourceID)
	if err != nil {
		return app.SettlementCandidate{}, err
	}
	if payment.Class != app.PixSettled || payment.ProviderStatus != "RECEIVED" {
		return app.SettlementCandidate{}, app.ErrNotSettled
	}
	settledAt := event.ReceivedAt.UTC()
	if settledAt.IsZero() {
		settledAt = time.Now().UTC()
	}
	channel := domain.ChannelPixManual
	orderID := payment.ExternalReference
	if !domain.ValidID(orderID) {
		orderID = ""
	}
	if payment.SubscriptionID != "" {
		channel = domain.ChannelPixAutomatic
		orderID = ""
	}
	if !payment.SettledAt.IsZero() {
		settledAt = payment.SettledAt
	}
	return app.SettlementCandidate{
		OrderID: orderID, Provider: domain.ProviderAsaas,
		Environment: p.Environment, Merchant: event.Merchant, Channel: channel,
		CustomerID: payment.CustomerID, RemotePaymentID: payment.ID,
		RemoteSubscriptionID: payment.SubscriptionID, ProviderStatus: payment.ProviderStatus,
		Amount: payment.Amount, SettledAt: settledAt,
	}, nil
}

func (p *AsaasProvider) CreateRefund(ctx context.Context, request app.RefundProviderRequest) (app.RefundProviderResult, error) {
	if !validAsaasID(request.RemotePaymentID) || request.Amount.Centavos <= 0 {
		return app.RefundProviderResult{}, domain.ErrInvalid
	}
	body := map[string]any{
		"value":       json.Number(request.Amount.String()),
		"description": refundDescription(request),
	}
	var payment asaasPayment
	if err := p.do(ctx, http.MethodPost, "/v3/payments/"+url.PathEscape(request.RemotePaymentID)+"/refund", body, &payment); err != nil {
		return app.RefundProviderResult{}, err
	}
	if payment.ID != request.RemotePaymentID {
		return app.RefundProviderResult{}, domain.ErrBinding
	}
	result, found, complete, err := p.FindRefund(ctx, request)
	if err != nil {
		return app.RefundProviderResult{}, err
	}
	if found {
		return result, nil
	}
	if !complete {
		return app.RefundProviderResult{}, app.ErrAmbiguous
	}
	return app.RefundProviderResult{
		ID: request.RefundID, State: domain.RefundPending, RawStatus: payment.Status,
		Amount: request.Amount, At: time.Now().UTC(),
	}, nil
}

func (p *AsaasProvider) FindRefund(ctx context.Context, request app.RefundProviderRequest) (app.RefundProviderResult, bool, bool, error) {
	if !validAsaasID(request.RemotePaymentID) {
		return app.RefundProviderResult{}, false, false, domain.ErrInvalid
	}
	var list asaasRefundList
	query := url.Values{"limit": {"100"}}
	if err := p.do(ctx, http.MethodGet, "/v3/payments/"+url.PathEscape(request.RemotePaymentID)+"/refunds?"+query.Encode(), nil, &list); err != nil {
		return app.RefundProviderResult{}, false, false, err
	}
	var matches []asaasRefund
	for _, refund := range list.Data {
		if refund.Description != refundDescription(request) {
			continue
		}
		amount, err := parseBRLMinor(refund.Value)
		if err != nil {
			return app.RefundProviderResult{}, false, false, domain.ErrBinding
		}
		if request.Amount.Centavos > 0 && amount != request.Amount.Centavos {
			return app.RefundProviderResult{}, false, false, domain.ErrBinding
		}
		matches = append(matches, refund)
	}
	if len(matches) > 1 {
		return app.RefundProviderResult{}, false, false, domain.ErrConflict
	}
	if len(matches) == 1 {
		return mapAsaasRefund(request, matches[0]), true, true, nil
	}
	return app.RefundProviderResult{}, false, !list.HasMore && list.TotalCount <= len(list.Data), nil
}

func (p *AsaasProvider) VerifyDispute(ctx context.Context, event app.InboxRecord) (app.DisputeFact, error) {
	switch event.EventType {
	case "PAYMENT_CHARGEBACK_REQUESTED", "PAYMENT_CHARGEBACK_DISPUTE", "PAYMENT_AWAITING_CHARGEBACK_REVERSAL":
	default:
		return app.DisputeFact{}, app.ErrIgnored
	}
	if event.ResourceType != "payment" || !validAsaasID(event.ResourceID) {
		return app.DisputeFact{}, domain.ErrBinding
	}
	payment, err := p.RetrieveAsaasPayment(ctx, event.ResourceID)
	if err != nil {
		return app.DisputeFact{}, err
	}
	if payment.ID != event.ResourceID {
		return app.DisputeFact{}, domain.ErrBinding
	}
	amount, err := parseBRLMinor(payment.Value)
	if err != nil || amount <= 0 {
		return app.DisputeFact{}, domain.ErrBinding
	}
	state := domain.DisputePending
	switch payment.Status {
	case "CHARGEBACK_REQUESTED":
		state = domain.DisputePending
	case "CHARGEBACK_DISPUTE", "AWAITING_CHARGEBACK_REVERSAL":
		state = domain.DisputeUnderReview
	default:
		if payment.Chargeback == nil {
			return app.DisputeFact{}, domain.ErrBinding
		}
		switch payment.Chargeback.Status {
		case "REQUESTED":
			state = domain.DisputePending
		case "IN_DISPUTE":
			state = domain.DisputeUnderReview
		case "DISPUTE_LOST":
			state = domain.DisputeLost
		case "REVERSED", "DONE":
			state = domain.DisputeClosed
		default:
			return app.DisputeFact{}, app.ErrIgnored
		}
	}
	opened := event.ReceivedAt.UTC()
	if opened.IsZero() {
		opened = time.Now().UTC()
	}
	return app.DisputeFact{
		Provider: domain.ProviderAsaas, Environment: p.Environment, Merchant: event.Merchant,
		RemoteDisputeID: "chargeback:" + payment.ID, RemotePaymentID: payment.ID,
		Amount: domain.BRL(amount), State: state,
		OpenedAt: opened, LastEventAt: opened,
	}, nil
}

func refundDescription(request app.RefundProviderRequest) string {
	if request.IdempotencyKey != "" {
		return request.IdempotencyKey
	}
	return "refund:" + request.RefundID
}

func mapAsaasRefund(request app.RefundProviderRequest, refund asaasRefund) app.RefundProviderResult {
	state := domain.RefundPending
	switch refund.Status {
	case "DONE":
		state = domain.RefundSucceeded
	case "PENDING", "AWAITING_CRITICAL_ACTION_AUTHORIZATION", "AWAITING_CUSTOMER_EXTERNAL_AUTHORIZATION":
		state = domain.RefundPending
	case "CANCELLED":
		state = domain.RefundFailed
	default:
		state = domain.RefundPending
	}
	at := time.Now().UTC()
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", refund.DateCreated, time.UTC); err == nil {
		at = parsed.UTC()
	}
	return app.RefundProviderResult{
		ID: request.RefundID, State: state, RawStatus: refund.Status,
		Amount: request.Amount, At: at,
	}
}

type asaasRefund struct {
	DateCreated string      `json:"dateCreated"`
	Status      string      `json:"status"`
	Value       json.Number `json:"value"`
	Description string      `json:"description"`
}

type asaasRefundList struct {
	Data       []asaasRefund `json:"data"`
	TotalCount int           `json:"totalCount"`
	HasMore    bool          `json:"hasMore"`
}

type asaasChargeback struct {
	Status string `json:"status"`
	Value  string `json:"value"`
}

func (p *AsaasProvider) RetrieveAsaasPayment(ctx context.Context, paymentID string) (asaasPayment, error) {
	var payment asaasPayment
	if err := p.do(ctx, http.MethodGet, "/v3/payments/"+url.PathEscape(paymentID), nil, &payment); err != nil {
		return asaasPayment{}, err
	}
	return payment, nil
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
		return app.ErrProviderUnauthorized
	case status == http.StatusTooManyRequests, status >= 500:
		return app.ErrAmbiguous
	default:
		return domain.ErrUnavailable
	}
}

func mapAsaasPayment(payment asaasPayment, location *time.Location) (app.PixPayment, error) {
	if payment.ID == "" || payment.Customer == "" ||
		payment.Status == "" || payment.Value.String() == "" {
		return app.PixPayment{}, domain.ErrBinding
	}
	amount, err := parseBRLMinor(payment.Value)
	if err != nil {
		return app.PixPayment{}, domain.ErrBinding
	}
	result := app.PixPayment{
		ID: payment.ID, CustomerID: payment.Customer, SubscriptionID: payment.Subscription,
		ExternalReference: payment.ExternalReference, Amount: domain.BRL(amount),
		Status: payment.Status, ProviderStatus: payment.Status,
		Class: asaasPixClass(payment.Status),
	}
	if payment.DueDate != "" {
		if due, err := time.ParseInLocation("2006-01-02", payment.DueDate, location); err == nil {
			result.DueDate = due.UTC()
		}
	}
	if payment.PaymentDate != "" {
		if settled, err := time.ParseInLocation("2006-01-02", payment.PaymentDate, location); err == nil {
			result.SettledAt = settled.UTC()
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
	ID                string           `json:"id"`
	Customer          string           `json:"customer"`
	Subscription      string           `json:"subscription"`
	Status            string           `json:"status"`
	ExternalReference string           `json:"externalReference"`
	Value             json.Number      `json:"value"`
	DueDate           string           `json:"dueDate"`
	PaymentDate       string           `json:"paymentDate"`
	Chargeback        *asaasChargeback `json:"chargeback"`
}

type asaasPaymentList struct {
	Data       []asaasPayment `json:"data"`
	TotalCount int            `json:"totalCount"`
	HasMore    bool           `json:"hasMore"`
}

type asaasPixAuto struct {
	ID                  string      `json:"id"`
	CustomerID          string      `json:"customerId"`
	ContractID          string      `json:"contractId"`
	Status              string      `json:"status"`
	SubscriptionID      string      `json:"subscriptionId"`
	PaymentCreationMode string      `json:"paymentCreationMode"`
	RetryPolicy         string      `json:"retryPolicy"`
	Value               json.Number `json:"value"`
	StartDate           string      `json:"startDate"`
	FinishDate          string      `json:"finishDate"`
	Payload             string      `json:"payload"`
	EncodedImage        string      `json:"encodedImage"`
	ImmediateQrCode     struct {
		ExpirationDate string `json:"expirationDate"`
	} `json:"immediateQrCode"`
}

type asaasPixAutoList struct {
	Data       []asaasPixAuto `json:"data"`
	TotalCount int            `json:"totalCount"`
	HasMore    bool           `json:"hasMore"`
}

type asaasPixAutoInstruction struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	DueDate       string `json:"dueDate"`
	PaymentID     string `json:"paymentId"`
	Purpose       string `json:"purpose"`
	RefusalReason string `json:"refusalReason"`
	RetryAttempt  int    `json:"retryAttempt"`
	Authorization struct {
		ID string `json:"id"`
	} `json:"authorization"`
}

type asaasPixAutoInstructionList struct {
	Data       []asaasPixAutoInstruction `json:"data"`
	TotalCount int                       `json:"totalCount"`
	HasMore    bool                      `json:"hasMore"`
}

func mapPixAutoInstruction(response asaasPixAutoInstruction) (app.PixAutoInstruction, error) {
	if response.ID == "" || response.Authorization.ID == "" || response.Status == "" ||
		response.Purpose == "" || response.DueDate == "" || len(response.RefusalReason) > 128 ||
		response.RetryAttempt < 0 || response.RetryAttempt > 3 ||
		(response.PaymentID != "" && !validAsaasID(response.PaymentID)) {
		return app.PixAutoInstruction{}, domain.ErrBinding
	}
	switch response.Status {
	case "AWAITING_REQUEST", "SCHEDULED", "DONE", "REFUSED", "CANCELLED":
	default:
		return app.PixAutoInstruction{}, domain.ErrBinding
	}
	if response.Purpose != "SCHEDULE" && response.Purpose != "RETRY_AFTER_DUE_DATE" {
		return app.PixAutoInstruction{}, domain.ErrBinding
	}
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return app.PixAutoInstruction{}, domain.ErrUnavailable
	}
	dueDate, err := time.ParseInLocation("2006-01-02", response.DueDate, location)
	if err != nil {
		return app.PixAutoInstruction{}, domain.ErrBinding
	}
	return app.PixAutoInstruction{
		ID: response.ID, AuthorizationID: response.Authorization.ID, PaymentID: response.PaymentID,
		Status: response.Status, Purpose: response.Purpose, RetryAttempt: response.RetryAttempt,
		DueDate: dueDate.UTC(), RefusalReason: response.RefusalReason,
	}, nil
}

func mapPixAuto(response asaasPixAuto, location *time.Location) (app.PixAutoAuthorization, error) {
	if response.ID == "" || response.CustomerID == "" || response.ContractID == "" || response.Status == "" ||
		response.Value.String() == "" || len(response.ContractID) > 35 ||
		(response.Payload != "" && len(response.Payload) > 4096) ||
		(response.EncodedImage != "" && len(response.EncodedImage) > 64*1024) {
		return app.PixAutoAuthorization{}, domain.ErrBinding
	}
	amount, err := parseBRLMinor(response.Value)
	if err != nil {
		return app.PixAutoAuthorization{}, domain.ErrBinding
	}
	retryPolicy := response.RetryPolicy
	if retryPolicy == "" {
		retryPolicy = "NOT_ALLOWED"
	}
	creationMode := response.PaymentCreationMode
	if creationMode == "" {
		creationMode = "SUBSCRIPTION"
	}
	result := app.PixAutoAuthorization{
		ID: response.ID, CustomerID: response.CustomerID, ContractID: response.ContractID,
		RemoteSubscriptionID: response.SubscriptionID, Status: response.Status,
		PaymentCreationMode: creationMode, RetryPolicy: retryPolicy,
		Value: domain.BRL(amount), QRCode: response.Payload, QRImage: response.EncodedImage,
	}
	if response.StartDate != "" {
		if value, err := time.ParseInLocation("2006-01-02", response.StartDate, location); err == nil {
			result.StartDate = value.UTC()
		}
	}
	if response.FinishDate != "" {
		if value, err := time.ParseInLocation("2006-01-02", response.FinishDate, location); err == nil {
			result.FinishDate = value.UTC()
		}
	}
	if response.ImmediateQrCode.ExpirationDate != "" {
		for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
			if value, err := time.ParseInLocation(layout, response.ImmediateQrCode.ExpirationDate, location); err == nil {
				expires := value.UTC()
				result.ExpiresAt = &expires
				break
			}
		}
	}
	return result, nil
}

type asaasQR struct {
	EncodedImage   string `json:"encodedImage"`
	Payload        string `json:"payload"`
	ExpirationDate string `json:"expirationDate"`
}

var (
	_ app.PixProvider     = (*AsaasProvider)(nil)
	_ app.PixAutoProvider = (*AsaasProvider)(nil)
)
