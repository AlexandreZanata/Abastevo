package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

const (
	LinkCreatePath   = Prefix + "/{document_id}/links/create"
	LinkListPath     = Prefix + "/{document_id}/links/list"
	LinkByTargetPath = Prefix + "/links/by-target"
)

// LinkRecords is the link routes' record gate (EDR-163): a link is created on
// an existing document, a document's links are read on that document, and a
// target's links are read from the subject's own attach authority on the
// organization root; the target side is checked in the link transaction.
type LinkRecords struct{}

func (LinkRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	root := scope.OrgID().String()
	switch policy {
	case app.AttachPolicy():
		if facts.ID == root || facts.AuthoredBy == "" {
			return access.ErrUnavailable
		}
	case app.LinkReadPolicy():
		if facts.ID == root && facts.AuthoredBy != subject.MembershipID() || facts.ID != root && facts.AuthoredBy == "" {
			return access.ErrUnavailable
		}
	default:
		return access.ErrUnavailable
	}
	return nil
}

type linkTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func (t *linkTarget) ref() app.TargetRef {
	return app.TargetRef{Kind: domain.TargetKind(t.Kind), ID: t.ID}
}

type linkCreateRequest struct {
	companyRequest
	SchemaVersion int         `json:"schema_version"`
	OperationID   string      `json:"operation_id"`
	Target        *linkTarget `json:"target"`
}

func (v linkCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type linkListRequest struct {
	companyRequest
	orgapi.Page
}

type linkByTargetRequest struct {
	companyRequest
	Target *linkTarget `json:"target"`
	orgapi.Page
}

// linkJSON exposes link metadata and the document's storage status only.
func linkJSON(v app.LinkView) map[string]any {
	return map[string]any{"document_id": v.DocumentID, "target": map[string]any{"kind": string(v.Target.Kind), "id": v.Target.ID},
		"document_storage": string(v.DocumentStorage), "created_at": instant(v.CreatedAt)}
}

func linkItems(page app.LinkPage) []map[string]any {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, linkJSON(item))
	}
	return items
}

// registerLinks mounts the three link routes when the link surface exists.
func registerLinks(owned chi.Router, entry orgapi.Gateway, service app.Service, authorization access.Authorization) error {
	if service.Links == nil {
		return nil
	}
	links := authorization
	links.Records = LinkRecords{}
	attach, read := app.AttachPolicy(), app.LinkReadPolicy()
	resource := documentResource{repository: service.Repository}
	if err := guarded.Register(owned, LinkCreatePath, entry, rule(attach, http.StatusCreated), links, attach, resource, func(ctx context.Context, p orgapi.Principal, _ access.Decision, input linkCreateRequest) (any, error) {
		id, ok := documentPathID(ctx)
		if !ok {
			return nil, orgapi.NotFound
		}
		if input.Target == nil {
			return nil, orgapi.Malformed
		}
		linked, err := service.Link(ctx, p.Scope, principalActor(p), app.Link{OperationID: input.OperationID, DocumentID: id, Target: input.Target.ref()})
		if err != nil {
			return nil, mapped(err, string(attach.Permission))
		}
		return map[string]any{"link": linkJSON(linked.View), "replayed": linked.Replayed}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, LinkListPath, entry, rule(read, http.StatusOK), links, read, resource, func(ctx context.Context, p orgapi.Principal, _ access.Decision, input linkListRequest) (any, error) {
		id, ok := documentPathID(ctx)
		if !ok {
			return nil, orgapi.NotFound
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("document.links.document.v1:%s:%d", id, page.Limit)
		after, err := entry.OpenCursor(p, LinkListPath, binding, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.DocumentLinkFilter{Limit: page.Limit}
		if after != "" {
			kind, target, found := strings.Cut(after, "/")
			if !found {
				return nil, orgapi.Malformed
			}
			filter.After = &app.TargetRef{Kind: domain.TargetKind(kind), ID: target}
		}
		links, err := service.DocumentLinks(ctx, p.Scope, principalActor(p), id, filter)
		if err != nil {
			return nil, mapped(err, string(read.Permission))
		}
		next := ""
		if links.More && len(links.Items) > 0 {
			last := links.Items[len(links.Items)-1].Target
			if next, err = entry.SealCursor(p, LinkListPath, binding, string(last.Kind)+"/"+last.ID); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": linkItems(links), "next_cursor": next}, nil
	}); err != nil {
		return err
	}
	return guarded.Register(owned, LinkByTargetPath, entry, rule(read, http.StatusOK), links, read, createResource{}, func(ctx context.Context, p orgapi.Principal, _ access.Decision, input linkByTargetRequest) (any, error) {
		if input.Target == nil {
			return nil, orgapi.Malformed
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		target := input.Target.ref()
		binding := fmt.Sprintf("document.links.target.v1:%s:%s:%d", target.Kind, target.ID, page.Limit)
		after, err := entry.OpenCursor(p, LinkByTargetPath, binding, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		links, err := service.TargetLinks(ctx, p.Scope, principalActor(p), target, app.TargetLinkFilter{Limit: page.Limit, AfterDocument: after})
		if err != nil {
			return nil, mapped(err, string(read.Permission))
		}
		next := ""
		if links.More && len(links.Items) > 0 {
			if next, err = entry.SealCursor(p, LinkByTargetPath, binding, links.Items[len(links.Items)-1].DocumentID); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": linkItems(links), "next_cursor": next}, nil
	})
}
