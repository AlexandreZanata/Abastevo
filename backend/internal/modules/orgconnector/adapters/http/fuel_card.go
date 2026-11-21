package http

import (
	"context"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	PreviewStatementPath = "/v1/orgs/{org_id}/integrations/connections/{connection_id}/fuel-card/preview"
	ConfirmStatementPath = "/v1/orgs/{org_id}/integrations/connections/{connection_id}/fuel-card/confirm"
)

type statementRequest struct {
	orgapi.Command
	SourceAccountID string `json:"source_account_id"`
	CSV             string `json:"csv"`
	PreviewToken    string `json:"preview_token,omitempty"`
}

type statementResponse struct {
	Rows         []app.StatementOutcome `json:"rows"`
	PreviewToken string                 `json:"preview_token,omitempty"`
	Replayed     bool                   `json:"replayed"`
}

func RegisterStatements(router chi.Router, service app.StatementCommands, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if service == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	for _, route := range []struct {
		path   string
		action app.Action
	}{{PreviewStatementPath, app.PreviewStatement}, {ConfirmStatementPath, app.ConfirmStatement}} {
		if err := register(owned, route.path, gateway, authorization, route.action,
			func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in statementRequest) (any, error) {
				if in.Revision != nil {
					return nil, orgapi.Malformed
				}
				input := app.Statement{ConnectionID: chi.URLParamFromCtx(ctx, "connection_id"), SourceAccountID: in.SourceAccountID, CSV: in.CSV}
				if route.action == app.ConfirmStatement {
					input.OperationID = in.OperationID
					proof, err := gateway.OpenCursor(p, ConfirmStatementPath, input.PayloadDigest(), in.PreviewToken)
					if err != nil || proof == "" {
						return nil, orgapi.Malformed
					}
					input.PreviewProof = proof
				} else if in.PreviewToken != "" {
					return nil, orgapi.Malformed
				}
				out, err := service.ProcessStatement(ctx, p.Scope, actor, p.MembershipID, route.action, input)
				if err != nil {
					return nil, connectorPolicyError(err, string(app.Policy(route.action).Permission))
				}
				response := statementResponse{Rows: out.Rows, Replayed: out.Replayed}
				if route.action == app.PreviewStatement {
					response.PreviewToken, err = gateway.SealCursor(p, ConfirmStatementPath, input.PayloadDigest(), out.Proof)
				}
				return response, err
			}); err != nil {
			return err
		}
	}
	return nil
}
