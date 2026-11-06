package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

const BulletinSourceCode = "abastevo-editorial"

// BulletinGroup identifies the newest bulletin for one survey-window group.
type BulletinGroup struct {
	PostID           string
	PublicationState domain.PublicationState
}

// BulletinAggregateReader is the Official-owned aggregate function shape.
type BulletinAggregateReader interface {
	ReadWeeklyBulletins(ctx context.Context, q officialapp.BulletinQuery) (officialapp.BulletinSet, error)
}

// BulletinStore persists deterministic drafts and correction linkage.
type BulletinStore interface {
	GetSourceByCode(ctx context.Context, code string) (*domain.Source, error)
	FindLatestBulletinGroup(ctx context.Context, groupKey string) (*BulletinGroup, error)
	CreateBulletinDraft(ctx context.Context, post *domain.Post) (bool, error)
}

// BulletinReport summarizes one bounded generator run.
type BulletinReport struct {
	Created  int
	Existing int
	Skipped  int
}

// BulletinGenerator builds pending-review drafts from accepted Official
// revision pairs. It never publishes and never rewrites Official facts.
type BulletinGenerator struct {
	Official BulletinAggregateReader
	Store    BulletinStore
	Clock    Clock
	IDs      IDGenerator
}

var bulletinFuelUnits = []struct {
	Fuel string
	Unit string
}{
	{"ETHANOL", "L"},
	{"GASOLINE_REGULAR", "L"},
	{"GASOLINE_ADDITIVED", "L"},
	{"GASOLINE_PREMIUM_GRADE", "L"},
	{"DIESEL_S500", "L"},
	{"DIESEL_S10", "L"},
	{"CNG", "M3"},
	{"LPG_P13", "KG_13"},
}

// Generate runs every official fuel/unit pair. A bounded no-pair source is a
// skip; corrupt input or missing Abastevo source fails visibly.
func (g BulletinGenerator) Generate(ctx context.Context) (BulletinReport, error) {
	if g.Official == nil || g.Store == nil || g.Clock == nil || g.IDs == nil {
		return BulletinReport{}, fmt.Errorf("forum: bulletin generator dependencies not initialized")
	}
	source, err := g.Store.GetSourceByCode(ctx, BulletinSourceCode)
	if err != nil {
		return BulletinReport{}, err
	}
	if source == nil || source.RightsStatus != domain.RightsApproved || !source.PublicationEnabled {
		return BulletinReport{}, domain.ErrMissingRights
	}

	report := BulletinReport{}
	for _, pair := range bulletinFuelUnits {
		set, err := g.Official.ReadWeeklyBulletins(ctx, officialapp.BulletinQuery{
			MethodologyVersion: officialapp.BulletinMethodologyVersion,
			Fuel:               pair.Fuel,
			Unit:               pair.Unit,
		})
		if errors.Is(err, officialapp.ErrBulletinUnavailable) {
			continue
		}
		if err != nil {
			return report, err
		}
		report.Skipped += len(set.Skipped)
		for _, pairSet := range set.Pairs {
			input := bulletinInputFromOfficial(set, pairSet)
			draft, err := domain.BuildBulletinDraft(input)
			if err != nil {
				return report, fmt.Errorf("forum: build bulletin %s/%s/%s: %w", pairSet.UF, pair.Fuel, pair.Unit, err)
			}
			latest, err := g.Store.FindLatestBulletinGroup(ctx, draft.Identity.GroupKey)
			if err != nil {
				return report, err
			}
			now := g.Clock.Now().UTC()
			sourceID := source.ID
			key := draft.Key
			groupKey := draft.Identity.GroupKey
			postID := g.IDs.NewID()
			if postID == "" {
				return report, fmt.Errorf("forum: bulletin id generator returned empty id")
			}
			post := &domain.Post{
				ID:                 postID,
				Kind:               domain.KindBulletin,
				Category:           domain.CategoryAbastevoData,
				Title:              draft.Title,
				Excerpt:            draft.Excerpt,
				BodyText:           draft.BodyText,
				SourceID:           sourceID,
				PublicationState:   domain.StatePendingReview,
				Visibility:         domain.VisibilityVisible,
				Revision:           1,
				CommentsEnabled:    true,
				Region:             &domain.Region{UF: pairSet.UF},
				BulletinKey:        &key,
				BulletinGroupKey:   &groupKey,
				BulletinProvenance: bulletinProvenanceFromOfficial(set, pairSet, draft),
				LocalCreatedAt:     now,
				LocalUpdatedAt:     now,
			}
			if latest != nil && latest.PostID != post.ID {
				post.SupersedesPostID = &latest.PostID
			}
			created, err := g.Store.CreateBulletinDraft(ctx, post)
			if err != nil {
				return report, err
			}
			if created {
				report.Created++
			} else {
				report.Existing++
			}
		}
	}
	return report, nil
}

func bulletinInputFromOfficial(set officialapp.BulletinSet, pair officialapp.BulletinPair) domain.BulletinInput {
	return domain.BulletinInput{
		UF:                         pair.UF,
		Fuel:                       pair.Fuel,
		Unit:                       pair.Unit,
		CurrentRevisionID:          set.CurrentRevision.RevisionID,
		CurrentSurveyStart:         set.CurrentRevision.SurveyStart,
		CurrentSurveyEnd:           set.CurrentRevision.SurveyEnd,
		CurrentPublishedAt:         set.CurrentRevision.PublishedAt,
		CurrentSourceURL:           set.CurrentRevision.SourceURL,
		CurrentSourceChecksum:      set.CurrentRevision.SourceChecksum,
		CurrentParserVersion:       set.CurrentRevision.ParserVersion,
		CurrentSumMilliBrl:         pair.Current.SumMilliBrl,
		CurrentObservationCount:    pair.Current.ObservationCount,
		CurrentSampledStations:     pair.Current.Coverage.SampledStations,
		CurrentSourceStations:      pair.Current.Coverage.SourceStations,
		CurrentSourceObservations:  pair.Current.Coverage.SourceObservations,
		PreviousRevisionID:         set.PreviousRevision.RevisionID,
		PreviousSurveyStart:        set.PreviousRevision.SurveyStart,
		PreviousSurveyEnd:          set.PreviousRevision.SurveyEnd,
		PreviousPublishedAt:        set.PreviousRevision.PublishedAt,
		PreviousSourceURL:          set.PreviousRevision.SourceURL,
		PreviousSourceChecksum:     set.PreviousRevision.SourceChecksum,
		PreviousParserVersion:      set.PreviousRevision.ParserVersion,
		PreviousSumMilliBrl:        pair.Previous.SumMilliBrl,
		PreviousObservationCount:   pair.Previous.ObservationCount,
		PreviousSampledStations:    pair.Previous.Coverage.SampledStations,
		PreviousSourceStations:     pair.Previous.Coverage.SourceStations,
		PreviousSourceObservations: pair.Previous.Coverage.SourceObservations,
	}
}

func bulletinProvenanceFromOfficial(set officialapp.BulletinSet, pair officialapp.BulletinPair, draft *domain.BulletinDraft) *domain.BulletinProvenance {
	return &domain.BulletinProvenance{
		MethodologyVersion: set.MethodologyVersion,
		TemplateVersion:    draft.TemplateVersion,
		Fuel:               pair.Fuel,
		Unit:               pair.Unit,
		Current:            bulletinRevisionProvenance(set.CurrentRevision, pair.Current),
		Previous:           bulletinRevisionProvenance(set.PreviousRevision, pair.Previous),
	}
}

func bulletinRevisionProvenance(revision officialapp.BulletinRevision, sample officialapp.BulletinSample) domain.BulletinRevisionProvenance {
	return domain.BulletinRevisionProvenance{
		RevisionID:         revision.RevisionID,
		SurveyStart:        revision.SurveyStart,
		SurveyEnd:          revision.SurveyEnd,
		PublishedAt:        revision.PublishedAt,
		SourceURL:          revision.SourceURL,
		SourceChecksum:     revision.SourceChecksum,
		ParserVersion:      revision.ParserVersion,
		SumMilliBrl:        sample.SumMilliBrl,
		ObservationCount:   sample.ObservationCount,
		SampledStations:    sample.Coverage.SampledStations,
		SourceObservations: sample.Coverage.SourceObservations,
		SourceStations:     sample.Coverage.SourceStations,
		MeanNumerator:      sample.MeanNumerator,
		MeanDenominator:    sample.MeanDenominator,
		Mean:               sample.Mean,
	}
}

// BulletinReaderFunc adapts the Official read function to the port.
type BulletinReaderFunc func(ctx context.Context, q officialapp.BulletinQuery) (officialapp.BulletinSet, error)

func (f BulletinReaderFunc) ReadWeeklyBulletins(ctx context.Context, q officialapp.BulletinQuery) (officialapp.BulletinSet, error) {
	return f(ctx, q)
}
