// Package jobs adapts durable digest-only subscription work to application ports.
package jobs

import (
	"context"
	"encoding/hex"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	platform "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type Handler struct {
	Name      string
	Run       func(context.Context, string) error
	RunWithID func(context.Context, string, string) error
}

func (h Handler) Kind() string { return h.Name }
func (h Handler) Version() int { return 1 }
func (h Handler) Handle(ctx context.Context, j platform.Job) error {
	var p struct {
		Version int    `json:"version"`
		Digest  string `json:"digest"`
	}
	if httpapi.DecodeJSON(j.Payload, &p) != nil || p.Version != 1 || len(p.Digest) != 64 || (h.Run == nil && h.RunWithID == nil) {
		return domain.ErrInvalid
	}
	if _, err := hex.DecodeString(p.Digest); err != nil {
		return domain.ErrInvalid
	}
	if h.RunWithID != nil {
		return h.RunWithID(ctx, p.Digest, j.ID)
	}
	return h.Run(ctx, p.Digest)
}
