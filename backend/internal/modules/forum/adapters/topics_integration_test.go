//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
)

func TestTopic_CreateAndIdempotency(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "motorista-sp")
	alias := "Motorista SP"
	uf := "SP"
	ibge := 3550308
	fuel := "GASOLINE_REGULAR"
	opID := "22222222-2222-4222-8222-000000000001"

	cmd := application.CreateTopicCommand{
		AuthorAccountID: authorID,
		PublicAlias:     &alias,
		Category:        domain.CategoryFuels,
		Title:           "Preço médio da gasolina na capital paulista",
		BodyText:        "Percebi uma variação considerável de R$ 5,49 a R$ 6,19 nos postos da Zona Sul esta semana. Como está na região de vocês?",
		Region: &domain.Region{
			UF:               uf,
			MunicipalityIBGE: &ibge,
		},
		FuelProduct: &fuel,
		OperationID: opID,
	}

	// 1. Initial creation
	res, err := store.CreateTopic(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error creating topic: %v", err)
	}
	if res.Replayed {
		t.Errorf("expected fresh creation, got replayed: true")
	}

	topic := res.Topic
	if topic.ID == "" {
		t.Errorf("expected non-empty topic ID")
	}
	if topic.Kind != domain.KindTopic {
		t.Errorf("expected kind TOPIC, got %s", topic.Kind)
	}
	if topic.Category != domain.CategoryFuels {
		t.Errorf("expected category FUELS, got %s", topic.Category)
	}
	if topic.Title != cmd.Title {
		t.Errorf("expected title %q, got %q", cmd.Title, topic.Title)
	}
	if topic.BodyText != cmd.BodyText {
		t.Errorf("expected body %q, got %q", cmd.BodyText, topic.BodyText)
	}
	if topic.SourceID != "" {
		t.Errorf("expected empty SourceID for user topic, got %q", topic.SourceID)
	}
	if topic.PublicationState != domain.StatePendingReview {
		t.Errorf("expected state PENDING_REVIEW, got %s", topic.PublicationState)
	}
	if topic.Visibility != domain.VisibilityVisible {
		t.Errorf("expected visibility VISIBLE, got %s", topic.Visibility)
	}
	if topic.Revision != 1 {
		t.Errorf("expected revision 1, got %d", topic.Revision)
	}
	if !topic.CommentsEnabled {
		t.Errorf("expected comments enabled for user topic")
	}
	if topic.Region == nil || topic.Region.UF != uf || topic.Region.MunicipalityIBGE == nil || *topic.Region.MunicipalityIBGE != ibge {
		t.Errorf("expected region SP / 3550308, got %+v", topic.Region)
	}
	if topic.FuelProduct == nil || *topic.FuelProduct != fuel {
		t.Errorf("expected fuel product GASOLINA_COMUM, got %v", topic.FuelProduct)
	}

	// Verify DB record directly
	var dbAuthorOp pgtype.UUID
	var dbKind, dbState string
	err = pool.QueryRow(ctx, `
		SELECT author_operation_id, kind, publication_state
		FROM forum_posts WHERE id = $1
	`, topic.ID).Scan(&dbAuthorOp, &dbKind, &dbState)
	if err != nil {
		t.Fatalf("failed querying topic from DB: %v", err)
	}
	if dbKind != "TOPIC" || dbState != "PENDING_REVIEW" {
		t.Errorf("unexpected db row values: kind=%s, state=%s", dbKind, dbState)
	}

	// 2. Exact replay: same operation ID and same payload
	replayRes, err := store.CreateTopic(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error replaying topic creation: %v", err)
	}
	if !replayRes.Replayed {
		t.Errorf("expected replayed: true on duplicate operation ID")
	}
	if replayRes.Topic.ID != topic.ID {
		t.Errorf("expected replayed topic ID %s, got %s", topic.ID, replayRes.Topic.ID)
	}

	// 3. Conflicting replay: same operation ID with conflicting title/body
	conflictingCmd := cmd
	conflictingCmd.Title = "Título completamente diferente na colisão de operation_id"
	_, err = store.CreateTopic(ctx, conflictingCmd)
	if !errors.Is(err, domain.ErrConflictingOperation) {
		t.Errorf("expected ErrConflictingOperation, got %v", err)
	}
}

func TestTopic_SimultaneousCreateRace(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "driver-race")
	opID := "22222222-2222-4222-8222-000000000002"

	cmd := application.CreateTopicCommand{
		AuthorAccountID: authorID,
		Category:        domain.CategoryVehiclesTech,
		Title:           "Eficiência do motor flex com proporções variadas de etanol",
		BodyText:        "Gostaria de discutir a curva de consumo energético quando misturamos 50% de gasolina e 50% de etanol no tanque flex.",
		OperationID:     opID,
	}

	const concurrentWorkers = 10
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	type result struct {
		res application.TopicResult
		err error
	}
	results := make([]result, concurrentWorkers)

	for i := 0; i < concurrentWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startBarrier
			r, err := store.CreateTopic(ctx, cmd)
			results[idx] = result{res: r, err: err}
		}(i)
	}

	close(startBarrier)
	wg.Wait()

	var freshCount, replayCount int
	var topicID string

	for idx, r := range results {
		if r.err != nil {
			t.Errorf("worker %d returned unexpected error: %v", idx, r.err)
			continue
		}
		if topicID == "" {
			topicID = r.res.Topic.ID
		} else if r.res.Topic.ID != topicID {
			t.Errorf("worker %d got different topic ID %s vs %s", idx, r.res.Topic.ID, topicID)
		}

		if r.res.Replayed {
			replayCount++
		} else {
			freshCount++
		}
	}

	if freshCount != 1 {
		t.Errorf("expected exactly 1 fresh insert, got %d", freshCount)
	}
	if replayCount != concurrentWorkers-1 {
		t.Errorf("expected %d replays, got %d", concurrentWorkers-1, replayCount)
	}

	// Verify DB has exactly one row
	var rowCount int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM forum_posts
		WHERE author_account_id = $1 AND author_operation_id = $2
	`, authorID, opID).Scan(&rowCount)
	if err != nil {
		t.Fatalf("failed counting posts: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("expected exactly 1 row in DB, got %d", rowCount)
	}
}

func TestTopic_LiveQuotaEnforcement(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "heavy-author")

	// Create 20 topics (maximum allowed live topics)
	var firstTopicID string
	for i := 1; i <= application.MaxLiveTopicsPerAccount; i++ {
		opID := fmt.Sprintf("22222222-2222-4222-8222-%012d", i)
		res, err := store.CreateTopic(ctx, application.CreateTopicCommand{
			AuthorAccountID: authorID,
			Category:        domain.CategoryTrafficRules,
			Title:           fmt.Sprintf("Dúvida de Trânsito #%d sobre nova sinalização", i),
			BodyText:        fmt.Sprintf("Texto da dúvida número %d referente à legislação de trânsito vigente e fiscalização por radares.", i),
			OperationID:     opID,
		})
		if err != nil {
			t.Fatalf("topic %d creation failed: %v", i, err)
		}
		if i == 1 {
			firstTopicID = res.Topic.ID
		}
	}

	// 21st topic must be rejected with ErrTopicLiveQuotaExceeded
	op21 := "22222222-2222-4222-8222-000000000021"
	_, err := store.CreateTopic(ctx, application.CreateTopicCommand{
		AuthorAccountID: authorID,
		Category:        domain.CategoryTrafficRules,
		Title:           "Tópico 21 que excede a cota de 20 tópicos ativos",
		BodyText:        "Este tópico deve ser rejeitado pois o usuário já atingiu 20 tópicos ativos na plataforma.",
		OperationID:     op21,
	})
	if !errors.Is(err, domain.ErrTopicLiveQuotaExceeded) {
		t.Fatalf("expected ErrTopicLiveQuotaExceeded on 21st topic, got: %v", err)
	}

	// Author removes 1 topic -> live count decreases to 19
	_, err = store.RemoveTopic(ctx, application.RemoveTopicCommand{
		TopicID:          firstTopicID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("failed removing topic 1: %v", err)
	}

	// Now creating 21st topic succeeds because live active count is 19
	res21, err := store.CreateTopic(ctx, application.CreateTopicCommand{
		AuthorAccountID: authorID,
		Category:        domain.CategoryTrafficRules,
		Title:           "Tópico 21 agora aceito após liberação de cota ativa",
		BodyText:        "Com a remoção do primeiro tópico, a cota de tópicos ativos voltou a ficar abaixo de 20.",
		OperationID:     op21,
	})
	if err != nil {
		t.Fatalf("expected topic creation to succeed after removal, got: %v", err)
	}
	if res21.Topic.ID == "" {
		t.Errorf("expected valid topic ID for 21st topic")
	}
}

func TestTopic_RateQuota(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	// Configure limiter with 3 writes per hour (B-BR-FOR-11 hourly topic quota)
	policy := identitydomain.QuotaPolicy{
		Version: 1,
		Ops: map[string]identitydomain.OperationQuota{
			identitydomain.OperationWrite: {Limit: 3, Window: time.Hour},
		},
	}
	limiter := identityadapters.NewLimiter(pool, policy)
	store.SetQuotaChecker(limiter)

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "rate-limited-author")

	// 3 topics within the hour -> OK
	for i := 1; i <= 3; i++ {
		opID := fmt.Sprintf("22222222-2222-4222-8222-%012d", i)
		_, err := store.CreateTopic(ctx, application.CreateTopicCommand{
			AuthorAccountID: authorID,
			Category:        domain.CategoryDriverEconomy,
			Title:           fmt.Sprintf("Dica de Economia #%d na condução urbana", i),
			BodyText:        fmt.Sprintf("Descrição da dica %d para manter o consumo reduzido em trajetos urbanos.", i),
			OperationID:     opID,
		})
		if err != nil {
			t.Fatalf("topic %d failed: %v", i, err)
		}
	}

	// 4th topic -> Exceeds hourly limit of 3 -> denied with QuotaError
	op4 := "22222222-2222-4222-8222-000000000004"
	_, err := store.CreateTopic(ctx, application.CreateTopicCommand{
		AuthorAccountID: authorID,
		Category:        domain.CategoryDriverEconomy,
		Title:           "Dica de Economia #4 além do limite por hora",
		BodyText:        "Esta quarta dica deve ser rejeitada pela política de limite por hora de tópicos.",
		OperationID:     op4,
	})
	if err == nil {
		t.Fatal("expected quota error on 4th topic, got nil")
	}

	var quotaErr *identityadapters.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Errorf("expected *identityadapters.QuotaError, got: %v", err)
	} else if quotaErr.RetryAfter <= 0 {
		t.Errorf("expected positive RetryAfter, got: %v", quotaErr.RetryAfter)
	}
}

func TestTopic_CASEditLifecycle(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-editor")

	createRes, err := store.CreateTopic(ctx, application.CreateTopicCommand{
		AuthorAccountID: authorID,
		Category:        domain.CategoryFuels,
		Title:           "Título Original do Tópico",
		BodyText:        "Texto original do tópico antes de qualquer edição pelo autor.",
		OperationID:     "22222222-2222-4222-8222-000000000001",
	})
	if err != nil {
		t.Fatalf("failed creating topic: %v", err)
	}
	topicID := createRes.Topic.ID

	// 1. Valid CAS Edit with matching ExpectedRevision = 1
	newTitle := "Título Atualizado do Tópico"
	newBody := "Corpo atualizado do tópico após a revisão do autor com mais detalhes."
	editRes, err := store.EditTopic(ctx, application.EditTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 1,
		Title:            newTitle,
		BodyText:         newBody,
		Category:         domain.CategoryFuels,
	})
	if err != nil {
		t.Fatalf("unexpected error editing topic: %v", err)
	}

	if editRes.Revision != 2 {
		t.Errorf("expected revision 2, got %d", editRes.Revision)
	}
	if editRes.Title != newTitle {
		t.Errorf("expected updated title %q, got %q", newTitle, editRes.Title)
	}
	if editRes.BodyText != newBody {
		t.Errorf("expected updated body %q, got %q", newBody, editRes.BodyText)
	}
	// Per B-BR-FOR-09: an edited topic returns to PENDING_REVIEW
	if editRes.PublicationState != domain.StatePendingReview {
		t.Errorf("expected state PENDING_REVIEW after edit, got %s", editRes.PublicationState)
	}

	// 2. Stale edit with old ExpectedRevision = 1 -> ErrRevisionMismatch
	_, err = store.EditTopic(ctx, application.EditTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 1,
		Title:            "Tentativa de edição com revisão desatualizada",
		BodyText:         "Esta edição usa revision=1 quando a revision atual no banco é 2.",
		Category:         domain.CategoryFuels,
	})
	if !errors.Is(err, domain.ErrRevisionMismatch) {
		t.Errorf("expected ErrRevisionMismatch on stale revision, got %v", err)
	}

	// 3. Edit concurrency race: 5 workers trying to CAS edit revision 2 simultaneously
	const editWorkers = 5
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	type editOutcome struct {
		topic *domain.Post
		err   error
	}
	outcomes := make([]editOutcome, editWorkers)

	for i := 0; i < editWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startBarrier
			res, err := store.EditTopic(ctx, application.EditTopicCommand{
				TopicID:          topicID,
				AuthorAccountID:  authorID,
				ExpectedRevision: 2,
				Title:            fmt.Sprintf("Título concorrente #%d", idx),
				BodyText:         fmt.Sprintf("Corpo concorrente #%d com detalhes.", idx),
				Category:         domain.CategoryFuels,
			})
			outcomes[idx] = editOutcome{topic: res, err: err}
		}(i)
	}

	close(startBarrier)
	wg.Wait()

	var successCount, mismatchCount int
	for _, o := range outcomes {
		if o.err == nil {
			successCount++
			if o.topic.Revision != 3 {
				t.Errorf("expected revision 3 for winning edit, got %d", o.topic.Revision)
			}
		} else if errors.Is(o.err, domain.ErrRevisionMismatch) {
			mismatchCount++
		} else {
			t.Errorf("unexpected error in concurrent edit: %v", o.err)
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful CAS edit, got %d", successCount)
	}
	if mismatchCount != editWorkers-1 {
		t.Errorf("expected %d mismatch errors, got %d", editWorkers-1, mismatchCount)
	}
}

func TestTopic_TombstoneRemoval(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-remover")
	alias := "Alias Removivel"

	createRes, err := store.CreateTopic(ctx, application.CreateTopicCommand{
		AuthorAccountID: authorID,
		PublicAlias:     &alias,
		Category:        domain.CategoryRoadsMobility,
		Title:           "Condições da Rodovia dos Imigrantes",
		BodyText:        "Operação subida em andamento com tráfego lento na serra.",
		OperationID:     "22222222-2222-4222-8222-000000000001",
	})
	if err != nil {
		t.Fatalf("failed creating topic: %v", err)
	}
	topicID := createRes.Topic.ID

	// 1. Remove topic with matching revision 1
	removeRes, err := store.RemoveTopic(ctx, application.RemoveTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error removing topic: %v", err)
	}

	if removeRes.PublicationState != domain.StateRemoved {
		t.Errorf("expected publication_state REMOVED, got %s", removeRes.PublicationState)
	}
	if removeRes.Visibility != domain.VisibilityHidden {
		t.Errorf("expected visibility HIDDEN, got %s", removeRes.Visibility)
	}
	if removeRes.Revision != 2 {
		t.Errorf("expected revision 2 after removal, got %d", removeRes.Revision)
	}
	if removeRes.BodyText != "" {
		t.Errorf("expected cleared body_text on tombstone, got %q", removeRes.BodyText)
	}
	if removeRes.PublicAlias != nil {
		t.Errorf("expected cleared public_alias on tombstone, got %v", removeRes.PublicAlias)
	}

	// Verify database record directly for tombstone state
	var dbTitle, dbBody string
	var dbAlias *string
	var dbState, dbVisibility string
	err = pool.QueryRow(ctx, `
		SELECT title, body_text, public_alias, publication_state, visibility
		FROM forum_posts WHERE id = $1
	`, topicID).Scan(&dbTitle, &dbBody, &dbAlias, &dbState, &dbVisibility)
	if err != nil {
		t.Fatalf("failed querying tombstoned post: %v", err)
	}
	if dbTitle != "[Tópico removido pelo autor]" {
		t.Errorf("expected tombstone title '[Tópico removido pelo autor]', got %q", dbTitle)
	}
	if dbBody != "" {
		t.Errorf("expected empty body_text in DB, got %q", dbBody)
	}
	if dbAlias != nil {
		t.Errorf("expected null public_alias in DB, got %v", *dbAlias)
	}
	if dbState != "REMOVED" || dbVisibility != "HIDDEN" {
		t.Errorf("expected state REMOVED/HIDDEN, got state=%s visibility=%s", dbState, dbVisibility)
	}

	// 2. Terminal removal: subsequent edit fails with ErrTopicAlreadyRemoved
	_, err = store.EditTopic(ctx, application.EditTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 2,
		Title:            "Tentativa de reativar tópico removido",
		BodyText:         "Não deve ser permitido editar tópicos após remoção pelo autor.",
		Category:         domain.CategoryRoadsMobility,
	})
	if !errors.Is(err, domain.ErrTopicAlreadyRemoved) {
		t.Errorf("expected ErrTopicAlreadyRemoved on edit after removal, got %v", err)
	}

	// 3. Terminal removal: subsequent remove fails with ErrTopicAlreadyRemoved
	_, err = store.RemoveTopic(ctx, application.RemoveTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 2,
	})
	if !errors.Is(err, domain.ErrTopicAlreadyRemoved) {
		t.Errorf("expected ErrTopicAlreadyRemoved on remove after removal, got %v", err)
	}
}

func TestTopic_ForeignOwnerDefense_IDOR(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorA := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "owner-a")
	authorB := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000002", "attacker-b")

	createRes, err := store.CreateTopic(ctx, application.CreateTopicCommand{
		AuthorAccountID: authorA,
		Category:        domain.CategoryFuels,
		Title:           "Tópico Privado do Autor A",
		BodyText:        "Apenas o autor A pode editar ou remover este tópico.",
		OperationID:     "22222222-2222-4222-8222-000000000001",
	})
	if err != nil {
		t.Fatalf("failed creating topic: %v", err)
	}
	topicID := createRes.Topic.ID

	// Attacker B attempts to edit Author A's topic -> ErrTopicAuthorMismatch
	_, err = store.EditTopic(ctx, application.EditTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorB,
		ExpectedRevision: 1,
		Title:            "Tentativa de alteração por usuário não autorizado",
		BodyText:         "Corpo forjado pelo usuário B tentando sequestrar o tópico.",
		Category:         domain.CategoryFuels,
	})
	if !errors.Is(err, domain.ErrTopicAuthorMismatch) {
		t.Errorf("expected ErrTopicAuthorMismatch on foreign edit, got %v", err)
	}

	// Attacker B attempts to remove Author A's topic -> ErrTopicAuthorMismatch
	_, err = store.RemoveTopic(ctx, application.RemoveTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorB,
		ExpectedRevision: 1,
	})
	if !errors.Is(err, domain.ErrTopicAuthorMismatch) {
		t.Errorf("expected ErrTopicAuthorMismatch on foreign removal, got %v", err)
	}
}

func TestTopic_GuardAccountStatus(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "suspended-author")

	// Set status to suspended in DB
	_, err := pool.Exec(ctx, `UPDATE accounts SET status = 'suspended' WHERE id = $1`, authorID)
	if err != nil {
		t.Fatalf("failed updating account status: %v", err)
	}

	errAccountSuspended := errors.New("auth: account suspended")
	// Guard simulating auth policy checking account status
	guardCheck := func(ctx context.Context, tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM accounts WHERE id = $1`, authorID).Scan(&status)
		if err != nil {
			return err
		}
		if status != "active" {
			return errAccountSuspended
		}
		return nil
	}

	_, err = store.CreateTopic(ctx, application.CreateTopicCommand{
		AuthorAccountID: authorID,
		Category:        domain.CategoryFuels,
		Title:           "Tópico de autor com conta suspensa",
		BodyText:        "Não deve ser aceito enquanto a conta estiver suspensa.",
		OperationID:     "22222222-2222-4222-8222-000000000001",
		Guard:           guardCheck,
	})
	if !errors.Is(err, errAccountSuspended) {
		t.Errorf("expected errAccountSuspended from guard, got %v", err)
	}

	// Verify no topic was inserted into forum_posts
	var postCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM forum_posts WHERE author_account_id = $1`, authorID).Scan(&postCount)
	if err != nil {
		t.Fatalf("failed querying post count: %v", err)
	}
	if postCount != 0 {
		t.Errorf("expected 0 posts after failed guard transaction, got %d", postCount)
	}
}

func TestTopic_KindSpoofAndConstraintDefenses(t *testing.T) {
	_, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "spoof-tester")

	// 1. Topic with source_id MUST fail check constraint forum_posts_source_editorial_ck
	_, err := pool.Exec(ctx, `
		INSERT INTO forum_posts (
			id, kind, category, title, body_text, source_id, author_account_id,
			publication_state, visibility, revision
		) VALUES (
			gen_random_uuid(), 'TOPIC', 'FUELS', 'Tópico Forjado com Fonte Externa',
			'Corpo tentando burlar proveniência com source_id externo.',
			'00000000-0000-0000-0000-000000000001', $1, 'PENDING_REVIEW', 'VISIBLE', 1
		)
	`, authorID)
	if err == nil {
		t.Error("expected DB constraint error when inserting TOPIC with non-null source_id, got nil")
	}

	// 2. Topic without author_account_id MUST fail check constraint forum_posts_author_presence_ck
	_, err = pool.Exec(ctx, `
		INSERT INTO forum_posts (
			id, kind, category, title, body_text, source_id, author_account_id,
			publication_state, visibility, revision
		) VALUES (
			gen_random_uuid(), 'TOPIC', 'FUELS', 'Tópico Anônimo Sem Autor',
			'Corpo tentando inserir tópico sem autor autenticado.',
			NULL, NULL, 'PENDING_REVIEW', 'VISIBLE', 1
		)
	`, authorID)
	if err == nil {
		t.Error("expected DB constraint error when inserting TOPIC with null author_account_id, got nil")
	}

	// 3. EDITORIAL without source_id MUST fail check constraint
	_, err = pool.Exec(ctx, `
		INSERT INTO forum_posts (
			id, kind, category, title, body_text, source_id, author_account_id,
			publication_state, visibility, revision
		) VALUES (
			gen_random_uuid(), 'EDITORIAL', 'FUELS', 'Editorial Sem Fonte',
			'Corpo editorial sem source_id.',
			NULL, $1, 'DRAFT', 'VISIBLE', 1
		)
	`, authorID)
	if err == nil {
		t.Error("expected DB constraint error when inserting EDITORIAL with null source_id, got nil")
	}
}

func TestTopic_AuditedForumChanges(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "audit-author")

	// 1. Create topic (rev 1)
	createRes, err := store.CreateTopic(ctx, application.CreateTopicCommand{
		AuthorAccountID: authorID,
		Category:        domain.CategoryDriverEconomy,
		Title:           "Planejamento de rotas para redução de consumo",
		BodyText:        "Dicas sobre uso de aplicativos de navegação para evitar vias congestionadas e manter velocidade constante.",
		OperationID:     "22222222-2222-4222-8222-000000000001",
	})
	if err != nil {
		t.Fatalf("create topic failed: %v", err)
	}
	topicID := createRes.Topic.ID

	// 2. Edit topic (rev 2)
	_, err = store.EditTopic(ctx, application.EditTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 1,
		Title:            "Planejamento de rotas para redução de consumo (Revisado)",
		BodyText:         "Dicas adicionais sobre uso de ar condicionado e calibragem dos pneus.",
		Category:         domain.CategoryDriverEconomy,
	})
	if err != nil {
		t.Fatalf("edit topic failed: %v", err)
	}

	// 3. Remove topic (rev 3)
	_, err = store.RemoveTopic(ctx, application.RemoveTopicCommand{
		TopicID:          topicID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 2,
	})
	if err != nil {
		t.Fatalf("remove topic failed: %v", err)
	}

	// Query forum_changes for this topic
	rows, err := pool.Query(ctx, `
		SELECT action, actor_kind, actor_account_id, revision, prior_state, new_state, reason_code
		FROM forum_changes
		WHERE target_type = 'POST' AND target_id = $1
		ORDER BY revision ASC
	`, topicID)
	if err != nil {
		t.Fatalf("failed querying forum_changes: %v", err)
	}
	defer rows.Close()

	type changeRow struct {
		action         string
		actorKind      string
		actorAccountID string
		revision       int32
		priorState     *string
		newState       *string
		reasonCode     string
	}
	var changes []changeRow

	for rows.Next() {
		var c changeRow
		var actorUUID pgtype.UUID
		if err := rows.Scan(&c.action, &c.actorKind, &actorUUID, &c.revision, &c.priorState, &c.newState, &c.reasonCode); err != nil {
			t.Fatalf("failed scanning forum change row: %v", err)
		}
		c.actorAccountID = fmt.Sprintf("%x-%x-%x-%x-%x", actorUUID.Bytes[0:4], actorUUID.Bytes[4:6], actorUUID.Bytes[6:8], actorUUID.Bytes[8:10], actorUUID.Bytes[10:16])
		changes = append(changes, c)
	}

	if len(changes) != 3 {
		t.Fatalf("expected 3 audit rows, got %d", len(changes))
	}

	// Row 1: CREATE
	if changes[0].action != "CREATE" || changes[0].revision != 1 || changes[0].actorKind != "AUTHOR" {
		t.Errorf("unexpected change row 1: %+v", changes[0])
	}
	if changes[0].newState == nil || *changes[0].newState != "PENDING_REVIEW" {
		t.Errorf("expected newState PENDING_REVIEW on CREATE, got %v", changes[0].newState)
	}

	// Row 2: EDIT
	if changes[1].action != "EDIT" || changes[1].revision != 2 || changes[1].actorKind != "AUTHOR" {
		t.Errorf("unexpected change row 2: %+v", changes[1])
	}
	if changes[1].newState == nil || *changes[1].newState != "PENDING_REVIEW" {
		t.Errorf("expected newState PENDING_REVIEW on EDIT, got %v", changes[1].newState)
	}

	// Row 3: REMOVE
	if changes[2].action != "REMOVE" || changes[2].revision != 3 || changes[2].actorKind != "AUTHOR" {
		t.Errorf("unexpected change row 3: %+v", changes[2])
	}
	if changes[2].reasonCode != "author-removal" {
		t.Errorf("expected reasonCode 'author-removal', got %q", changes[2].reasonCode)
	}
	if changes[2].newState == nil || *changes[2].newState != "REMOVED" {
		t.Errorf("expected newState REMOVED on REMOVE, got %v", changes[2].newState)
	}
}
