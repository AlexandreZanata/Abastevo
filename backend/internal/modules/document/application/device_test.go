package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

const (
	deviceContributor = "7b0e2a54-0000-4000-8000-000000041531"
	deviceKey         = "7b0e2a54-0000-4000-8000-000000041532"
)

func deviceActor() app.Actor {
	return app.Actor{AccountID: accountID, MembershipID: memberID, Channel: "android", ContributorID: deviceContributor, DeviceKeyID: deviceKey}
}

func deviceUpload() app.DeviceUpload {
	return app.DeviceUpload{Create: command(), Link: link()}
}

type orderedLinks struct {
	fakeLinks
	order *[]string
}

func (r *orderedLinks) Link(ctx context.Context, scope kernel.OrgScope, actor app.Actor, l app.Link) (app.LinkView, bool, error) {
	*r.order = append(*r.order, "link")
	return r.fakeLinks.Link(ctx, scope, actor, l)
}

type orderedRepository struct {
	fakeRepository
	order *[]string
}

func (r *orderedRepository) Create(ctx context.Context, scope kernel.OrgScope, actor app.Actor, c app.Create) (app.View, bool, error) {
	*r.order = append(*r.order, "create")
	return r.fakeRepository.Create(ctx, scope, actor, c)
}

func deviceService(repository app.Repository, links app.LinkRepository, tokens *fakeTokens) app.Service {
	return app.Service{Enabled: true, Repository: repository, Links: links, Tokens: tokens, Now: func() time.Time { return now }}
}

// B-BR-EMP-417: declare, then link, and only then hand out the capability.
func TestDeviceSessionDeclaresThenLinksThenGrantsTheUpload(t *testing.T) {
	var order []string
	repository := &orderedRepository{fakeRepository: fakeRepository{created: view(domain.Uploading)}, order: &order}
	links := &orderedLinks{order: &order}
	tokens := &fakeTokens{}
	session, err := deviceService(repository, links, tokens).OpenDevice(t.Context(), scope(t), deviceActor(), deviceUpload())
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "create" || order[1] != "link" {
		t.Fatal("device session must declare before linking", order)
	}
	if session.View.ID != docID || session.View.Statuses.Storage != domain.Uploading || session.Upload == nil || session.Upload.Token == "" || tokens.issued != 1 {
		t.Fatal("an UPLOADING declaration must answer its fresh capability")
	}
	if !session.Upload.ExpiresAt.After(now) || session.Upload.ExpiresAt.After(now.Add(app.UploadTTL)) {
		t.Fatal("device capability outlives five minutes")
	}
}

// A refused link (wrong record, another member's record, another organization)
// never releases the capability; the declaration is idempotent and closes on
// its own (B-BR-EMP-254).
func TestDeviceSessionWithholdsTheCapabilityWhenTheLinkIsRefused(t *testing.T) {
	for _, refusal := range []error{app.ErrNotFound, permissions.DenyPermission("meter.reading.record"), app.ErrConflict, app.ErrOperationReused, app.ErrInvalid, app.ErrUnavailable} {
		repository := &fakeRepository{created: view(domain.Uploading)}
		links := &fakeLinks{err: refusal}
		session, err := deviceService(repository, links, &fakeTokens{}).OpenDevice(t.Context(), scope(t), deviceActor(), deviceUpload())
		if !errors.Is(err, refusal) || session.Upload != nil || session.View.ID != "" {
			t.Fatal("refused link released a session", refusal, err)
		}
		if repository.creates != 1 || links.links != 1 {
			t.Fatal("refused link was not attempted after the declaration")
		}
	}
}

// A terminal document answers its state without a link or capability; a
// verified or received one answers its state after the (replayed) link.
func TestDeviceSessionAnswersStoredAndTerminalStates(t *testing.T) {
	for _, terminal := range []domain.State{domain.StorageFailed, domain.Replaced, domain.Deleted} {
		links := &fakeLinks{}
		session, err := deviceService(&fakeRepository{created: view(terminal)}, links, &fakeTokens{}).OpenDevice(t.Context(), scope(t), deviceActor(), deviceUpload())
		if err != nil || session.View.Statuses.Storage != terminal || session.Upload != nil || links.links != 0 {
			t.Fatal("terminal document was linked or granted", terminal, err)
		}
	}
	for _, state := range []domain.State{domain.Received, domain.Stored, domain.Quarantined} {
		links := &fakeLinks{replayed: true}
		tokens := &fakeTokens{}
		session, err := deviceService(&fakeRepository{created: view(state), replayed: true}, links, tokens).OpenDevice(t.Context(), scope(t), deviceActor(), deviceUpload())
		if err != nil || session.View.Statuses.Storage != state || session.Upload != nil || links.links != 1 || tokens.issued != 0 {
			t.Fatal("non-uploading document granted bytes or skipped its link", state, err)
		}
	}
}

// Only the enrolled device plane opens device sessions, and the whole
// command is validated before any write.
func TestDeviceSessionValidatesActorAndCommandBeforeAnyWrite(t *testing.T) {
	web := actor()
	cases := []struct {
		name    string
		actor   app.Actor
		command func(app.DeviceUpload) app.DeviceUpload
		want    error
	}{
		{"web actor", web, func(c app.DeviceUpload) app.DeviceUpload { return c }, app.ErrNotFound},
		{"other document", deviceActor(), func(c app.DeviceUpload) app.DeviceUpload {
			c.Link.DocumentID = "7b0e2a54-0000-4000-8000-000000041599"
			return c
		}, app.ErrInvalid},
		{"declaration reused as link", deviceActor(), func(c app.DeviceUpload) app.DeviceUpload { c.Link.OperationID = c.Create.OperationID; return c }, app.ErrInvalid},
		{"document as link", deviceActor(), func(c app.DeviceUpload) app.DeviceUpload { c.Link.OperationID = c.Create.DocumentID; return c }, app.ErrInvalid},
		{"png", deviceActor(), func(c app.DeviceUpload) app.DeviceUpload { c.Create.Original.MediaType = "image/png"; return c }, app.ErrInvalid},
		{"oversized", deviceActor(), func(c app.DeviceUpload) app.DeviceUpload { c.Create.Original.Bytes = 5*1024*1024 + 1; return c }, app.ErrInvalid},
		{"bad target", deviceActor(), func(c app.DeviceUpload) app.DeviceUpload { c.Link.Target.ID = "not-a-uuid"; return c }, app.ErrInvalid},
		{"bad kind", deviceActor(), func(c app.DeviceUpload) app.DeviceUpload { c.Link.Target.Kind = "UNKNOWN"; return c }, app.ErrInvalid},
	}
	for _, test := range cases {
		repository := &fakeRepository{created: view(domain.Uploading)}
		links := &fakeLinks{}
		_, err := deviceService(repository, links, &fakeTokens{}).OpenDevice(t.Context(), scope(t), test.actor, test.command(deviceUpload()))
		if !errors.Is(err, test.want) || repository.creates != 0 || links.links != 0 {
			t.Fatal("device command reached storage", test.name, err)
		}
	}
	disabled := deviceService(&fakeRepository{}, &fakeLinks{}, &fakeTokens{})
	disabled.Enabled = false
	if _, err := disabled.OpenDevice(t.Context(), scope(t), deviceActor(), deviceUpload()); !errors.Is(err, app.ErrDisabled) {
		t.Fatal("default-off service opened a device session", err)
	}
	noLinks := deviceService(&fakeRepository{created: view(domain.Uploading)}, nil, &fakeTokens{})
	if _, err := noLinks.OpenDevice(t.Context(), scope(t), deviceActor(), deviceUpload()); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("device session without the link surface", err)
	}
	declared := &fakeRepository{createErr: app.ErrOperationReused}
	links := &fakeLinks{}
	if _, err := deviceService(declared, links, &fakeTokens{}).OpenDevice(t.Context(), scope(t), deviceActor(), deviceUpload()); !errors.Is(err, app.ErrOperationReused) || links.links != 0 {
		t.Fatal("refused declaration still linked", err)
	}
}

func TestDeviceValuesNeverFormatIdentifiers(t *testing.T) {
	session := app.DeviceSession{View: view(domain.Uploading), Upload: &app.UploadAccess{Token: "v1.synthetic-upload-capability"}}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, untimedLog))
	for _, value := range []any{deviceUpload(), session} {
		raw, _ := json.Marshal(value)
		logger.Info("synthetic", "value", value)
		for _, out := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(raw), logs.String()} {
			for _, secret := range []string{docID, readingID, linkOp, memberID, "synthetic-upload-capability"} {
				if strings.Contains(out, secret) {
					t.Fatal("private device value formatted", out)
				}
			}
		}
	}
}
