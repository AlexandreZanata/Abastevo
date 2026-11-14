package adapters

import (
	"context"
	"fmt"

	forumsql "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/forum"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// EditorialExportReader implements application.EditorialExportReader over
// the owned generated queries. It runs on a pool or inside a caller-owned
// read-only transaction, so the posts and the export snapshot share one view.
type EditorialExportReader struct {
	db forumsql.DBTX
}

// NewEditorialExportReader wires the owned queries to a pool or transaction.
func NewEditorialExportReader(db forumsql.DBTX) *EditorialExportReader {
	return &EditorialExportReader{db: db}
}

var _ application.EditorialExportReader = (*EditorialExportReader)(nil)

// EditorialFingerprint returns the change detector of the editorial web
// projection (B-BR-SEO-08).
func (r *EditorialExportReader) EditorialFingerprint(ctx context.Context) (application.EditorialExportFingerprint, error) {
	row, err := forumsql.New(r.db).ExportEditorialFingerprint(ctx)
	if err != nil {
		return application.EditorialExportFingerprint{}, fmt.Errorf("forum: editorial export fingerprint: %w", err)
	}
	return application.EditorialExportFingerprint{Posts: row.Posts, Hash: "md5:" + row.PostsHash}, nil
}

// ExportEditorialPage walks the editorial web projection in post id order.
func (r *EditorialExportReader) ExportEditorialPage(ctx context.Context, afterID string, limit int) ([]application.EditorialExportPost, string, error) {
	if err := application.ValidateEditorialExportPage(afterID, limit); err != nil {
		return nil, "", err
	}
	rows, err := forumsql.New(r.db).ExportEditorialPosts(ctx, forumsql.ExportEditorialPostsParams{
		AfterID: afterID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, "", fmt.Errorf("forum: editorial export page: %w", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]application.EditorialExportPost, 0, len(rows))
	for _, row := range rows {
		if !row.FirstPublishedAt.Valid || !row.LocalUpdatedAt.Valid {
			return nil, "", fmt.Errorf("forum: published editorial post without publication clock")
		}
		category := domain.Category(row.Category)
		name, description := application.EditorialCategory(category)
		post := application.EditorialExportPost{
			ID: uuidString(row.ID), Category: category,
			CategoryName: name, CategoryDescription: description,
			Title: row.Title, Excerpt: row.Excerpt, BodyText: row.BodyText,
			PublishedAt: row.FirstPublishedAt.Time.UTC(), UpdatedAt: row.LocalUpdatedAt.Time.UTC(),
			SourceName: row.SourceName,
		}
		if row.RegionUf.Valid {
			region := &domain.Region{UF: row.RegionUf.String}
			if row.RegionMunicipalityIbge.Valid {
				code := int(row.RegionMunicipalityIbge.Int32)
				region.MunicipalityIBGE = &code
			}
			post.Region = region
		}
		if row.FuelProduct.Valid {
			fuel := row.FuelProduct.String
			post.FuelProduct = &fuel
		}
		if row.StationID.Valid {
			station := uuidString(row.StationID)
			post.StationID = &station
		}
		out = append(out, post)
	}
	next := ""
	if hasMore && len(out) > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}
