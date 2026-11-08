package application

import (
	"context"
	"fmt"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// TopicService coordinates user topic authoring and lifecycle use cases.
type TopicService struct {
	store TopicStore
	clock Clock
}

// NewTopicService creates a new TopicService.
func NewTopicService(store TopicStore, clock Clock) *TopicService {
	if clock == nil {
		clock = SystemClock{}
	}
	return &TopicService{
		store: store,
		clock: clock,
	}
}

// CreateTopic validates topic metadata and creates or replays an authored topic.
func (s *TopicService) CreateTopic(ctx context.Context, cmd CreateTopicCommand) (TopicResult, error) {
	if cmd.AuthorAccountID == "" {
		return TopicResult{}, domain.ErrMissingAuthor
	}
	if cmd.OperationID == "" {
		return TopicResult{}, fmt.Errorf("forum: missing operation id")
	}

	var uf string
	var ibge *int
	if cmd.Region != nil {
		uf = cmd.Region.UF
		ibge = cmd.Region.MunicipalityIBGE
	}

	if err := domain.ValidateTopic(cmd.Category, cmd.Title, cmd.BodyText, uf, ibge, cmd.FuelProduct, cmd.StationID); err != nil {
		return TopicResult{}, err
	}

	cleanTitle, err := domain.ValidateTopicTitle(cmd.Title)
	if err != nil {
		return TopicResult{}, err
	}
	cmd.Title = cleanTitle

	cleanBody, err := domain.ValidateTopicBody(cmd.BodyText)
	if err != nil {
		return TopicResult{}, err
	}
	cmd.BodyText = cleanBody

	return s.store.CreateTopic(ctx, cmd)
}

// EditTopic validates input and updates topic content under an optimistic concurrency CAS check.
func (s *TopicService) EditTopic(ctx context.Context, cmd EditTopicCommand) (*domain.Post, error) {
	if cmd.TopicID == "" {
		return nil, domain.ErrTopicNotFound
	}
	if cmd.AuthorAccountID == "" {
		return nil, domain.ErrMissingAuthor
	}
	if cmd.ExpectedRevision < 1 {
		return nil, domain.ErrRevisionMismatch
	}

	var uf string
	var ibge *int
	if cmd.Region != nil {
		uf = cmd.Region.UF
		ibge = cmd.Region.MunicipalityIBGE
	}

	if err := domain.ValidateTopic(cmd.Category, cmd.Title, cmd.BodyText, uf, ibge, cmd.FuelProduct, cmd.StationID); err != nil {
		return nil, err
	}

	cleanTitle, err := domain.ValidateTopicTitle(cmd.Title)
	if err != nil {
		return nil, err
	}
	cmd.Title = cleanTitle

	cleanBody, err := domain.ValidateTopicBody(cmd.BodyText)
	if err != nil {
		return nil, err
	}
	cmd.BodyText = cleanBody

	return s.store.EditTopic(ctx, cmd)
}

// RemoveTopic performs an authenticated tombstone removal on an authored topic.
func (s *TopicService) RemoveTopic(ctx context.Context, cmd RemoveTopicCommand) (*domain.Post, error) {
	if cmd.TopicID == "" {
		return nil, domain.ErrTopicNotFound
	}
	if cmd.AuthorAccountID == "" {
		return nil, domain.ErrMissingAuthor
	}
	if cmd.ExpectedRevision < 1 {
		return nil, domain.ErrRevisionMismatch
	}

	return s.store.RemoveTopic(ctx, cmd)
}

// GetTopicByID retrieves a topic by its UUID.
func (s *TopicService) GetTopicByID(ctx context.Context, id string) (*domain.Post, error) {
	if id == "" {
		return nil, domain.ErrTopicNotFound
	}
	return s.store.GetTopicByID(ctx, id)
}
