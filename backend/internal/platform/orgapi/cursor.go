package orgapi

import (
	"encoding/json"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

type Page struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

func (page Page) PageFields() Page { return page }

func (page Page) Normalize() (Page, error) {
	if page.Limit == 0 {
		page.Limit = httpapi.DefaultLimit
	}
	if page.Limit < 1 || page.Limit > httpapi.MaxLimit || len(page.Cursor) > 2048 {
		return Page{}, Malformed
	}
	return page, nil
}

func (gateway Gateway) cursorBinding(principal Principal, route, filters string) (string, error) {
	if !validOrigin(gateway.Origin) || len(gateway.CursorSecret) < 32 ||
		principal.Scope.Validate() != nil || !validID(principal.AccountID) || !validID(principal.MembershipID) ||
		!IsOrganizationPath(route) || len(route) > 512 || len(filters) > 8192 {
		return "", Malformed
	}
	// JSON framing keeps embedded separators in validated filters unambiguous.
	binding, _ := json.Marshal([]string{"orgapi.cursor.v1", gateway.Origin,
		principal.Scope.OrgID().String(), principal.AccountID, principal.MembershipID, route, filters})
	return httpapi.FilterHash(string(binding)), nil
}

func (gateway Gateway) now() time.Time {
	if gateway.Now != nil {
		return gateway.Now()
	}
	return time.Now()
}

func (gateway Gateway) SealCursor(principal Principal, route, filters, lastKey string) (string, error) {
	binding, err := gateway.cursorBinding(principal, route, filters)
	if err != nil || lastKey == "" || len(lastKey) > 512 {
		return "", Malformed
	}
	return httpapi.Seal(gateway.CursorSecret, binding, lastKey, gateway.now()), nil
}

func (gateway Gateway) OpenCursor(principal Principal, route, filters, token string) (string, error) {
	binding, err := gateway.cursorBinding(principal, route, filters)
	if err != nil || len(token) > 2048 {
		return "", Malformed
	}
	if token == "" {
		return "", nil
	}
	key, err := httpapi.Open(gateway.CursorSecret, token, binding, gateway.now())
	if err != nil || key == "" || len(key) > 512 {
		return "", Malformed
	}
	return key, nil
}
