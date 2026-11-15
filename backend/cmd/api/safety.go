package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	feedbackadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/adapters"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	safetyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/adapters"
	safetyhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/adapters/http"
	safetyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/application"
	safetydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/domain"
	subscriptiondomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

// safetyDeps are the already-built collaborators the community safety
// routes compose (B-BR-SAFE-01…09). Only this composition root joins the
// safety module to forum, feedback, moderation, identity and accounts.
type safetyDeps struct {
	Pool        *pgxpool.Pool
	Forum       *forumadapters.Store
	Feedback    *feedbackadapters.PGStore
	Moderation  *moderationadapters.Store
	Accounts    *accountapp.Service
	AccountRows *accountadapters.PGStore
	SignedOwner func(r *http.Request, accountID string) (string, string, error)
}

// newSafetyStore builds the safety store with its author sources, case
// opener and new-block window limiter.
func newSafetyStore(d safetyDeps) *safetyadapters.Store {
	store := safetyadapters.NewStore(d.Pool)
	store.Sources = map[safetydomain.ContentKind]safetyadapters.AuthorSource{
		safetydomain.KindForumPost:      {PublicAuthorTx: d.Forum.SafetyTopicAuthorTx, Authors: d.Forum.SafetyTopicAuthors},
		safetydomain.KindForumComment:   {PublicAuthorTx: d.Forum.SafetyCommentAuthorTx, Authors: d.Forum.SafetyCommentAuthors},
		safetydomain.KindStationComment: {PublicAuthorTx: d.Feedback.SafetyCommentAuthorTx, Authors: d.Feedback.SafetyCommentAuthors},
	}
	store.OpenCase = func(ctx context.Context, tx pgx.Tx, targetType, targetID, reason, detail string) (string, bool, error) {
		caseID, err := newUUID()
		if err != nil {
			return "", false, err
		}
		c, _, err := moderationdomain.NewCase(moderationdomain.CaseParams{
			ID: caseID, TargetType: targetType, TargetID: targetID,
			Priority: moderationdomain.PriorityP2, Reason: reason, Detail: detail,
			OpenedAt: time.Now(), PolicyVersion: moderationdomain.PolicyV1,
		})
		if err != nil {
			return "", false, err
		}
		return d.Moderation.OpenCaseTx(ctx, tx, c)
	}
	// B-BR-SAFE-05: 50 new blocks per account per 24-hour window, consumed
	// in the block transaction so a rollback returns the admission.
	limiter := identityadapters.NewLimiter(d.Pool, identitydomain.QuotaPolicy{
		Version: 1,
		Ops: map[string]identitydomain.OperationQuota{identitydomain.OperationWrite: {
			Limit: safetydomain.BlockDailyQuota, Window: 24 * time.Hour,
		}},
	})
	store.BlockQuota = func(ctx context.Context, tx pgx.Tx, accountID string) error {
		digest := sha256.Sum256([]byte("safety-block-daily:" + accountID))
		retry, err := limiter.CheckTx(ctx, tx, hex.EncodeToString(digest[:]), identitydomain.OperationWrite)
		var quota *identityadapters.QuotaError
		if errors.As(err, &quota) {
			return &safetydomain.QuotaError{RetryAfterSeconds: int(math.Ceil(retry.Seconds()))}
		}
		return err
	}
	return store
}

// registerCommunitySafety mounts the signed safety routes.
func registerCommunitySafety(r chi.Router, d safetyDeps, store *safetyadapters.Store) {
	safetyhttp.Handler{
		Service: safetyapp.Service{Store: store},
		Sessions: func(ctx context.Context, family, access string) (string, error) {
			return d.Accounts.ValidateAccess(ctx, family, access)
		},
		SignedOwner: d.SignedOwner,
		Guard: func(ctx context.Context, tx pgx.Tx, owner, family, access, contributor, key string) error {
			if err := d.AccountRows.RequireQualitySessionTx(ctx, tx, owner, family, access, contributor, d.Accounts.Hasher, time.Now().Unix()); err != nil {
				return safetyGuardError(err)
			}
			return safetyGuardError(identityadapters.RequireQualitySigningKeyTx(ctx, tx, contributor, key))
		},
	}.RegisterRoutes(r)
}

// safetyGuardError reports refused credentials as 401 and keeps
// dependency failures retryable (503) without revealing internals.
func safetyGuardError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, subscriptiondomain.ErrUnauthorized) {
		return safetyhttp.ErrGuardRefused
	}
	switch accountdomain.VerdictCode(err) {
	case "invalid-value", "oidc-unavailable", "key-unavailable":
		return err
	default:
		return safetyhttp.ErrGuardRefused
	}
}
