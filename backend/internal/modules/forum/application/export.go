package application

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// ForumExportFormat versions the canonical export envelope.
const ForumExportFormat = "forum-export-v1"

// ForumTopicExport represents one exported topic row in the archive.
// Verbatim text with its revision, state and audit timestamps, never other authors' rows.
type ForumTopicExport struct {
	ID                     string  `json:"id"`
	Category               string  `json:"category"`
	Title                  string  `json:"title"`
	Excerpt                string  `json:"excerpt"`
	BodyText               string  `json:"body_text"`
	Revision               int     `json:"revision"`
	PublicationState       string  `json:"publication_state"`
	Visibility             string  `json:"visibility"`
	RegionUf               *string `json:"region_uf,omitempty"`
	RegionMunicipalityIbge *int    `json:"region_municipality_ibge,omitempty"`
	FuelProduct            *string `json:"fuel_product,omitempty"`
	StationID              *string `json:"station_id,omitempty"`
	CreatedAt              string  `json:"created_at"`
	UpdatedAt              string  `json:"updated_at"`
	PublishedAt            *string `json:"published_at,omitempty"`
}

// ForumCommentExport represents one exported comment/reply row in the archive.
// Verbatim text with its revision, state and audit timestamps, never other authors' rows.
type ForumCommentExport struct {
	ID               string  `json:"id"`
	PostID           string  `json:"post_id"`
	ParentID         *string `json:"parent_id,omitempty"`
	Depth            int16   `json:"depth"`
	Sequence         int64   `json:"sequence"`
	Revision         int     `json:"revision"`
	PublicationState string  `json:"publication_state"`
	Visibility       string  `json:"visibility"`
	BodyText         string  `json:"body_text"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	PublishedAt      *string `json:"published_at,omitempty"`
}

// ForumAccountFootprint is the owner's footprint across forum discussions.
type ForumAccountFootprint struct {
	AccountID     string               `json:"account_id"`
	Topics        []ForumTopicExport   `json:"topics"`
	TotalTopics   int                  `json:"total_topics"`
	Comments      []ForumCommentExport `json:"comments"`
	TotalComments int                  `json:"total_comments"`
}

// ForumExportEnvelope is the canonical export archive envelope.
type ForumExportEnvelope struct {
	Format      string                `json:"format"`
	AccountID   string                `json:"account_id"`
	GeneratedAt string                `json:"generated_at"`
	Footprint   ForumAccountFootprint `json:"footprint"`
}

type topicExporter interface {
	ListTopicsByAccount(ctx context.Context, accountID string) ([]domain.Post, error)
}

// ExportAccount assembles one author's forum footprint (LGPD right to access).
// Reads are strictly scoped to the requesting account ID by construction:
// topics and comments authored by other accounts are never returned or exported.
func ExportAccount(ctx context.Context, store CommentStore, accountID string, now time.Time) (ForumExportEnvelope, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ForumExportEnvelope{}, domain.ErrTargetInvalid
	}
	comments, err := store.ListCommentsByAccount(ctx, accountID)
	if err != nil {
		return ForumExportEnvelope{}, err
	}

	exported := make([]ForumCommentExport, 0, len(comments))
	for _, c := range comments {
		// Defense-in-depth: guarantee no foreign comment leaks even if store has a bug
		if c.AuthorAccountID != accountID {
			continue
		}
		var pubAt *string
		if c.PublishedAt != nil {
			s := c.PublishedAt.UTC().Format(time.RFC3339)
			pubAt = &s
		}
		exported = append(exported, ForumCommentExport{
			ID:               c.ID,
			PostID:           c.PostID,
			ParentID:         c.ParentID,
			Depth:            c.Depth,
			Sequence:         c.Sequence,
			Revision:         c.Revision,
			PublicationState: string(c.PublicationState),
			Visibility:       string(c.Visibility),
			BodyText:         c.BodyText,
			CreatedAt:        c.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:        c.UpdatedAt.UTC().Format(time.RFC3339),
			PublishedAt:      pubAt,
		})
	}

	sort.Slice(exported, func(i, j int) bool {
		return exported[i].ID < exported[j].ID
	})

	var exportedTopics []ForumTopicExport
	if te, ok := store.(topicExporter); ok {
		topics, err := te.ListTopicsByAccount(ctx, accountID)
		if err != nil {
			return ForumExportEnvelope{}, err
		}
		exportedTopics = make([]ForumTopicExport, 0, len(topics))
		for _, top := range topics {
			if top.AuthorAccountID != accountID {
				continue
			}
			var pubAt *string
			if top.FirstPublishedAt != nil {
				s := top.FirstPublishedAt.UTC().Format(time.RFC3339)
				pubAt = &s
			}
			var regUF *string
			var regIBGE *int
			if top.Region != nil {
				if top.Region.UF != "" {
					u := top.Region.UF
					regUF = &u
				}
				regIBGE = top.Region.MunicipalityIBGE
			}
			exportedTopics = append(exportedTopics, ForumTopicExport{
				ID:                     top.ID,
				Category:               string(top.Category),
				Title:                  top.Title,
				Excerpt:                top.Excerpt,
				BodyText:               top.BodyText,
				Revision:               top.Revision,
				PublicationState:       string(top.PublicationState),
				Visibility:             string(top.Visibility),
				RegionUf:               regUF,
				RegionMunicipalityIbge: regIBGE,
				FuelProduct:            top.FuelProduct,
				StationID:              top.StationID,
				CreatedAt:              top.LocalCreatedAt.UTC().Format(time.RFC3339),
				UpdatedAt:              top.LocalUpdatedAt.UTC().Format(time.RFC3339),
				PublishedAt:            pubAt,
			})
		}
		sort.Slice(exportedTopics, func(i, j int) bool {
			return exportedTopics[i].ID < exportedTopics[j].ID
		})
	}
	if exportedTopics == nil {
		exportedTopics = []ForumTopicExport{}
	}

	return ForumExportEnvelope{
		Format:      ForumExportFormat,
		AccountID:   accountID,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Footprint: ForumAccountFootprint{
			AccountID:     accountID,
			Topics:        exportedTopics,
			TotalTopics:   len(exportedTopics),
			Comments:      exported,
			TotalComments: len(exported),
		},
	}, nil
}

// ExportAccountBytes returns the deterministic JSON bytes for one author's forum footprint.
func ExportAccountBytes(ctx context.Context, store CommentStore, accountID string, now time.Time) ([]byte, error) {
	env, err := ExportAccount(ctx, store, accountID, now)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}
