package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// Editorial web export bounds (B-BR-FOR-19 / B-BR-SEO-08).
const (
	MinEditorialExportLimit = 1
	MaxEditorialExportLimit = 500
)

// ErrInvalidEditorialExportPage rejects an out-of-range page size or a
// malformed keyset cursor.
var ErrInvalidEditorialExportPage = errors.New("forum: invalid editorial export page")

// EditorialExportPost is the public web projection of one published, visible
// EDITORIAL post of an approved Abastevo editorial source. It carries only
// fields the anonymous read API already serves; author and account data never
// enter it.
type EditorialExportPost struct {
	ID                  string
	Category            domain.Category
	CategoryName        string
	CategoryDescription string
	Title               string
	Excerpt             string
	BodyText            string
	PublishedAt         time.Time
	UpdatedAt           time.Time
	Region              *domain.Region
	FuelProduct         *string
	StationID           *string
	SourceName          string
}

// EditorialExportFingerprint is a cheap change detector over every exported
// field of the editorial web projection.
type EditorialExportFingerprint struct {
	Posts int64
	Hash  string
}

// EditorialExportReader is the forum-owned port the enterprise export reads
// inside its own read-only transaction. Pages walk post ids in ascending
// order; the next cursor is empty on the last page.
type EditorialExportReader interface {
	EditorialFingerprint(ctx context.Context) (EditorialExportFingerprint, error)
	ExportEditorialPage(ctx context.Context, afterID string, limit int) ([]EditorialExportPost, string, error)
}

// ValidateEditorialExportPage bounds one export page request.
func ValidateEditorialExportPage(afterID string, limit int) error {
	if limit < MinEditorialExportLimit || limit > MaxEditorialExportLimit {
		return ErrInvalidEditorialExportPage
	}
	if afterID != "" && !canonicalUUID(afterID) {
		return ErrInvalidEditorialExportPage
	}
	return nil
}

// EditorialCategory resolves the code-owned category presentation used by the
// web projection; unknown codes keep the code as their name.
func EditorialCategory(code domain.Category) (name, description string) {
	for _, item := range domain.DefaultCategories() {
		if item.Code == code {
			return item.Name, item.Description
		}
	}
	return string(code), ""
}

func canonicalUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
	}
	return true
}
