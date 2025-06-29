package postgres_test

import (
	"context"
	"database/sql"

	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/config"
	"github.com/RogueAlmond70/Muzz/internal/datastore/postgres"

	"go.uber.org/zap"
)

func setupPostgres(t *testing.T) (*postgres.Postgres, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	cfg := config.Config{
		PageSize:      2,
		PostgresRetry: 1,
		PostgresDelay: time.Millisecond,
	}
	logger, _ := zap.NewDevelopment()
	p := postgres.NewPostgres(logger, db, cfg)
	return p, mock, func() {
		db.Close()
		logger.Sync()
	}
}

func TestGetLikes_NoCursor_Success(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user123",
	}

	// Mock rows returned from query
	rows := sqlmock.NewRows([]string{"liker_id", "created_at", "id"}).
		AddRow("likerA", time.Now(), 1).
		AddRow("likerB", time.Now().Add(-time.Minute), 2).
		AddRow("likerC", time.Now().Add(-2*time.Minute), 3) // extra row triggers nextCursor

	mock.ExpectQuery(regexp.QuoteMeta(`
	WITH latest_likes AS (
	SELECT DISTINCT ON (liker_id) liker_id, created_at, id
	FROM likes
	WHERE liked_id = $1 AND decision = true
	AND created_at >= NOW() - INTERVAL '1 year'
	ORDER BY liker_id, created_at DESC, id DESC
	)
	SELECT liker_id, created_at, id
	FROM latest_likes 
	 ORDER BY created_at DESC, id DESC LIMIT $2`)).
		WithArgs(req.RecipientUserId, 3).
		WillReturnRows(rows)

	resp, err := p.GetLikes(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Likers) != 2 {
		t.Errorf("expected 2 likers, got %d", len(resp.Likers))
	}
	if resp.NextPaginationToken == nil || *resp.NextPaginationToken == "" {
		t.Errorf("expected next pagination token to be set")
	}
}

func TestGetCountLikedYou_Success(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.CountLikedYouRequest{
		RecipientUserId: "user123",
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(DISTINCT l.liker_id)
		FROM likes l
		WHERE l.liked_id = $1 AND decision
		AND NOT EXISTS (
		SELECT 1
		FROM likes l2
		WHERE l2.liker_id = $1
		AND l2.liked_id = l.liker_id
		AND l2.decision = true
		AND created_at >= NOW() - INTERVAL '1 year'

	)`)).
		WithArgs(req.RecipientUserId).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

	resp, err := p.GetCountLikedYou(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 42 {
		t.Errorf("expected count 42, got %d", resp.Count)
	}
}

func TestPutDecision_NewDecisionMutualLike(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.PutDecisionRequest{
		ActorUserId:     "actor1",
		RecipientUserId: "recipient1",
		LikedRecipient:  true,
	}

	// Mock insert/update query returns true (decision updated)
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO likes (liker_id, liked_id, decision, decision_at, created_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (liker_id, liked_id)
		DO UPDATE SET 
			decision = EXCLUDED.decision,
			decision_at = EXCLUDED.decision_at
		WHERE likes.decision IS DISTINCT FROM EXCLUDED.decision
		RETURNING decision
	`)).
		WithArgs(req.ActorUserId, req.RecipientUserId, req.LikedRecipient).
		WillReturnRows(sqlmock.NewRows([]string{"decision"}).AddRow(true))

	// Mock check for mutual like: returns true
	mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT TRUE
			FROM likes
			WHERE liker_id = $1 AND liked_id = $2 AND decision = TRUE
			LIMIT 1
		`)).
		WithArgs(req.RecipientUserId, req.ActorUserId).
		WillReturnRows(sqlmock.NewRows([]string{"TRUE"}).AddRow(true))

	resp, err := p.PutDecision(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.MutualLikes {
		t.Errorf("expected mutual likes true")
	}
}

func TestPutDecision_NoMutualLike(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.PutDecisionRequest{
		ActorUserId:     "actor1",
		RecipientUserId: "recipient1",
		LikedRecipient:  true,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO likes (liker_id, liked_id, decision, decision_at, created_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (liker_id, liked_id)
		DO UPDATE SET 
			decision = EXCLUDED.decision,
			decision_at = EXCLUDED.decision_at
		WHERE likes.decision IS DISTINCT FROM EXCLUDED.decision
		RETURNING decision
	`)).
		WithArgs(req.ActorUserId, req.RecipientUserId, req.LikedRecipient).
		WillReturnRows(sqlmock.NewRows([]string{"decision"}).AddRow(true))

	// No mutual like found - sql.ErrNoRows returned
	mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT TRUE
			FROM likes
			WHERE liker_id = $1 AND liked_id = $2 AND decision = TRUE
			LIMIT 1
		`)).
		WithArgs(req.RecipientUserId, req.ActorUserId).
		WillReturnError(sql.ErrNoRows)

	resp, err := p.PutDecision(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MutualLikes {
		t.Errorf("expected mutual likes false")
	}
}
