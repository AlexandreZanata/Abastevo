package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	evidencestorage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

// evidenceURLTTL is the privileged moderation download window
// (SECURITY_PRIVACY): short-lived, owner-independent, never logged.
const evidenceURLTTL = 60 * time.Second

type evidenceArgs struct {
	operator   string
	caseID     string
	evidenceID string
}

func parseEvidenceArgs(args []string) (evidenceArgs, error) {
	fs := flag.NewFlagSet("evidence-url", flag.ContinueOnError)
	var e evidenceArgs
	fs.StringVar(&e.operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&e.caseID, "case", "", "open moderation case referencing the evidence")
	fs.StringVar(&e.evidenceID, "evidence-id", "", "evidence object ID bound to the case")
	if err := fs.Parse(args); err != nil {
		return evidenceArgs{}, err
	}
	if strings.TrimSpace(e.caseID) == "" || strings.TrimSpace(e.evidenceID) == "" {
		return evidenceArgs{}, errors.New("evidence-url requires --case and --evidence-id")
	}
	return e, nil
}

// evidenceAccessAllowed binds a privileged download to its audit case:
// the case must be actionable and must reference exactly the requested
// evidence, so an operator cannot fish arbitrary objects through any
// open case (B-BR-010/011).
func evidenceAccessAllowed(c moderationdomain.Case, evidenceID string) error {
	if c.Status != moderationdomain.StatusOpen && c.Status != moderationdomain.StatusInReview {
		return errors.New("evidence download requires an actionable case")
	}
	if strings.TrimSpace(evidenceID) == "" || c.EvidenceID == "" || c.EvidenceID != strings.TrimSpace(evidenceID) {
		return errors.New("evidence is not bound to this case")
	}
	return nil
}

// runEvidenceURL mints one 60 s owner-independent download for the
// evidence bound to an actionable case. The audit record carries actor,
// case, evidence and timestamp only: the signed URL itself is printed
// to stdout for the operator terminal and never stored or logged.
func runEvidenceURL(args []string, getenv func(string) string) error {
	parsed, err := parseEvidenceArgs(args)
	if err != nil {
		return err
	}
	operator, err := resolveOperatorID(parsed.operator, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, cfg, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	raw := pool.Underlying()
	modStore := moderationadapters.NewStore(raw)
	c, err := modStore.Get(ctx, strings.TrimSpace(parsed.caseID))
	if err != nil {
		return err
	}
	if err := evidenceAccessAllowed(c, parsed.evidenceID); err != nil {
		return err
	}
	uid, err := parseUUID(strings.TrimSpace(parsed.evidenceID))
	if err != nil {
		return err
	}
	obj, err := evidence.New(raw).GetObject(ctx, uid)
	if err != nil {
		return fmt.Errorf("evidence object: %w", err)
	}
	if obj.FinalDeletedAt.Valid {
		return errors.New("evidence unavailable: object deleted")
	}
	if cfg.R2 == nil {
		return errors.New("evidence storage not configured")
	}
	pre, err := evidencestorage.PresignGET(evidencestorage.PresignInput{
		Endpoint: cfg.R2.Endpoint, Bucket: cfg.R2.Bucket,
		Key: obj.FinalKey, Namespace: "f/",
		TTL: evidenceURLTTL, Region: cfg.R2.Region, Now: time.Now(),
	}, evidencestorage.Credentials{
		AccessKeyID: cfg.R2.AccessKeyID, SecretAccessKey: cfg.R2.SecretAccessKey,
	})
	if err != nil {
		return err
	}
	// Audit without the URL: actor, case, evidence, timestamp.
	fmt.Fprintf(os.Stderr, "evidence-access operator=%s case=%s evidence=%s at=%s\n",
		operator, c.ID, strings.TrimSpace(parsed.evidenceID), time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintln(os.Stdout, pre.URL)
	return nil
}

// parseUUID parses canonical UUID text for query parameters.
func parseUUID(text string) (pgtype.UUID, error) {
	clean := make([]byte, 0, len(text))
	for i := 0; i < len(text); i++ {
		if text[i] != '-' {
			clean = append(clean, text[i])
		}
	}
	raw, err := hex.DecodeString(string(clean))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("ops: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}
