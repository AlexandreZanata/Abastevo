package http

import (
	"context"
	"net/http"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type companyProofKey struct{}
type companyProof struct{ actor app.CompanyActor }

// Bind verified cookie metadata only to this request. AUTH still owns the
// current authority decision; a row ID alone never supplies that authority.
func companyEntry(router chi.Router, cookies company.Handler, gateway orgapi.Gateway) (chi.Router, orgapi.Gateway, error) {
	if cookies.Service == nil || !cookies.Service.Enabled || gateway.Sessions != nil || gateway.SignedOwner != nil || gateway.WebSession != nil || gateway.WebSessionConfirm != nil {
		return nil, orgapi.Gateway{}, app.ErrUnavailable
	}
	gateway.WebSession = cookies.WebSession
	gateway.WebSessionConfirm = func(r *http.Request, fresh bool) (string, error) {
		proof, _ := r.Context().Value(companyProofKey{}).(*companyProof)
		if proof == nil {
			return "", orgapi.Unavailable
		}
		row, err := cookies.ConfirmSession(r, fresh)
		if err != nil {
			return "", err
		}
		proof.actor = app.CompanyActor{AccountID: row.AccountID, SessionID: row.ID}
		return row.AccountID, nil
	}
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), companyProofKey{}, &companyProof{})))
		})
	}
	return router.With(middleware), gateway, nil
}
func companyActor(ctx context.Context) (app.CompanyActor, error) {
	proof, _ := ctx.Value(companyProofKey{}).(*companyProof)
	if proof == nil || proof.actor.Validate() != nil {
		return app.CompanyActor{}, orgapi.Unauthenticated
	}
	return proof.actor, nil
}

// CompanyEntry and CurrentCompanyActor expose only the verified company-cookie
// request boundary for other private modules. AUTH decisions remain separate.
func CompanyEntry(router chi.Router, cookies company.Handler, gateway orgapi.Gateway) (chi.Router, orgapi.Gateway, error) {
	return companyEntry(router, cookies, gateway)
}
func CurrentCompanyActor(ctx context.Context) (app.CompanyActor, error) { return companyActor(ctx) }
