// Package jobs adapts durable digest-only subscription work to application ports.
package jobs

import (
	"context"
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	platform "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type Handler struct {
	Name string
	Run  func(context.Context, string) error
}

func (h Handler) Kind() string { return h.Name }
func (h Handler) Version() int { return 1 }
func (h Handler) Handle(ctx context.Context, j platform.Job) error {
	var p struct {
		Version int    `json:"version"`
		Digest  string `json:"digest"`
	}
	if json.Unmarshal(j.Payload, &p) != nil || p.Version != 1 || len(p.Digest) != 64 || h.Run == nil {
		return domain.ErrInvalid
	}
	return h.Run(ctx, p.Digest)
}
